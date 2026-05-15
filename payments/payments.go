package payments

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/api"
	"github.com/BNCryptoBros/klv-consensus-monitor/config"
	"github.com/BNCryptoBros/klv-consensus-monitor/models"
	"github.com/BNCryptoBros/klv-consensus-monitor/price"
	klproto "github.com/klever-io/klever-go-sdk/models/proto"
	"github.com/klever-io/klever-go-sdk/provider/tools/hasher"
	"github.com/klever-io/klever-go-sdk/provider/tools/marshal"
)

const (
	klvPrecision           = 6
	klvAtomicUnitsPerUnit  = 1_000_000
	contractTypeTransfer   = uint32(0)
	contractTypeClaim      = uint32(9)
	claimTypeStaking       = int32(0)
	claimTypeAllowance     = int32(1)
	assetKLV               = "KLV"
)

type Generator struct {
	cfg         *config.Config
	apiClient   *api.Client
	priceClient *price.Client
	httpClient  *http.Client
	marshalizer marshal.Marshalizer
	hasher      hasher.Hasher
	outputDir   string
	dryRun      bool
}

func NewGenerator(cfg *config.Config, apiClient *api.Client, dryRun bool) *Generator {
	return &Generator{
		cfg:         cfg,
		apiClient:   apiClient,
		priceClient: price.NewClient(),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		marshalizer: &marshal.ProtoMarshalizer{},
		hasher:      &hasher.Blake2b{},
		dryRun:      dryRun,
	}
}

type ValidatorPlan struct {
	Validator       models.MonitoredValidator
	OwnerAddress    string
	Balance         int64
	Nonce           uint64
	StakingRewards  int64
	Allowance       int64
	BalanceFloor    int64
	InfraShare      int64
	PerWallet       int64
	DustToFirst     int64
	WalletPayouts   []walletPayout
	InfraAddress    string
	InfraNickname   string
	Skipped         bool
	SkipReason      string
}

type walletPayout struct {
	Wallet models.ValidatorWallet
	Amount int64
}

type Plan struct {
	Validators      []*ValidatorPlan
	KLVBRLPrice     float64
	InfraTotalBRL   float64
	InfraTotalKLV   int64
	BalanceFloor    int64
	GeneratedAt     time.Time
}

func (g *Generator) Run() error {
	if err := g.cfg.ValidatePayouts(); err != nil {
		return fmt.Errorf("invalid payouts config: %w", err)
	}

	plan, err := g.BuildPlan()
	if err != nil {
		return fmt.Errorf("build plan: %w", err)
	}

	g.PrintPlan(plan)

	stamp := plan.GeneratedAt.UTC().Format("20060102T150405Z")
	g.outputDir = filepath.Join("payouts", stamp)
	if err := os.MkdirAll(g.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	failures := 0
	for _, vp := range plan.Validators {
		if vp.Skipped {
			log.Printf("skipping %s (%s): %s", vp.Validator.DisplayName, vp.OwnerAddress, vp.SkipReason)
			continue
		}
		if err := g.processValidator(vp); err != nil {
			failures++
			log.Printf("FAILED to process %s: %v", vp.Validator.DisplayName, err)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d validator transaction(s) failed", failures)
	}
	return nil
}

func (g *Generator) BuildPlan() (*Plan, error) {
	if err := g.ensureOwnerAddresses(); err != nil {
		return nil, err
	}

	plan := &Plan{
		BalanceFloor: g.cfg.Payouts.BalanceFloor,
		GeneratedAt:  time.Now().UTC(),
	}

	plan.InfraTotalBRL = float64(g.cfg.Payouts.InfraCost.AmountBRLCents) / 100.0
	if g.cfg.Payouts.InfraCost.AmountBRLCents > 0 {
		priceBRLPerKLV, err := g.priceClient.FetchKLVPerBRL(
			g.cfg.Payouts.InfraCost.PriceAPIURL,
			g.cfg.Payouts.InfraCost.PriceJSONPath,
		)
		if err != nil {
			return nil, fmt.Errorf("fetch klv/brl price: %w", err)
		}
		plan.KLVBRLPrice = priceBRLPerKLV
		infraKLV := plan.InfraTotalBRL / priceBRLPerKLV
		plan.InfraTotalKLV = int64(math.Ceil(infraKLV * klvAtomicUnitsPerUnit))
	}

	plans, err := g.collectValidatorPlans()
	if err != nil {
		return nil, err
	}
	plan.Validators = plans

	solvent := 0
	for _, vp := range plans {
		if !vp.Skipped {
			solvent++
		}
	}

	var perValidatorInfra int64
	if solvent > 0 && plan.InfraTotalKLV > 0 {
		perValidatorInfra = int64(math.Ceil(float64(plan.InfraTotalKLV) / float64(solvent)))
	}

	numWallets := int64(len(g.cfg.Payouts.ValidatorWallets))
	for _, vp := range plans {
		if vp.Skipped {
			continue
		}
		available := vp.Balance + vp.StakingRewards + vp.Allowance - vp.BalanceFloor
		if available <= 0 {
			vp.Skipped = true
			vp.SkipReason = fmt.Sprintf("available <= 0 (balance=%d, staking=%d, allowance=%d, floor=%d)",
				vp.Balance, vp.StakingRewards, vp.Allowance, vp.BalanceFloor)
			continue
		}
		if perValidatorInfra > available {
			vp.Skipped = true
			vp.SkipReason = fmt.Sprintf("available %d cannot cover infra share %d", available, perValidatorInfra)
			continue
		}

		vp.InfraShare = perValidatorInfra
		vp.InfraAddress = g.cfg.Payouts.InfraCost.ManagerAddress
		vp.InfraNickname = g.cfg.Payouts.InfraCost.ManagerNickname

		walletPool := available - perValidatorInfra
		perWallet := int64(0)
		if numWallets > 0 {
			perWallet = walletPool / numWallets
		}
		vp.PerWallet = perWallet
		distributed := perWallet * numWallets
		vp.DustToFirst = walletPool - distributed

		vp.WalletPayouts = make([]walletPayout, 0, numWallets)
		for i, w := range g.cfg.Payouts.ValidatorWallets {
			amount := perWallet
			if i == 0 {
				amount += vp.DustToFirst
			}
			if amount <= 0 {
				continue
			}
			vp.WalletPayouts = append(vp.WalletPayouts, walletPayout{Wallet: w, Amount: amount})
		}
	}

	return plan, nil
}

func (g *Generator) ensureOwnerAddresses() error {
	for i := range g.cfg.Validators {
		v := &g.cfg.Validators[i]
		if v.OwnerAddress != "" {
			continue
		}
		info, err := g.apiClient.FetchValidatorByBLS(v.BLSKey)
		if err != nil {
			return fmt.Errorf("resolve owner for %s: %w", v.DisplayName, err)
		}
		v.OwnerAddress = info.OwnerAddress
		log.Printf("resolved %s → %s", v.DisplayName, info.OwnerAddress)
	}
	return nil
}

func (g *Generator) collectValidatorPlans() ([]*ValidatorPlan, error) {
	out := make([]*ValidatorPlan, 0, len(g.cfg.Validators))
	for i := range g.cfg.Validators {
		v := g.cfg.Validators[i]
		vp := &ValidatorPlan{
			Validator:    v,
			OwnerAddress: v.OwnerAddress,
			BalanceFloor: g.cfg.Payouts.BalanceFloor,
		}

		acct, err := g.apiClient.FetchAccountInfo(v.OwnerAddress)
		if err != nil {
			return nil, fmt.Errorf("account info for %s (%s): %w", v.DisplayName, v.OwnerAddress, err)
		}
		vp.Balance = acct.Data.Account.Balance
		vp.Nonce = acct.Data.Account.Nonce

		claim, err := g.apiClient.FetchAvailableClaim(v.OwnerAddress, assetKLV)
		if err != nil {
			return nil, fmt.Errorf("allowance for %s (%s): %w", v.DisplayName, v.OwnerAddress, err)
		}
		vp.StakingRewards = claim.Data.StakingRewards
		vp.Allowance = claim.Data.Allowance

		out = append(out, vp)
	}
	return out, nil
}

func (g *Generator) PrintPlan(plan *Plan) {
	log.Printf("=== Payment plan (generated %s) ===", plan.GeneratedAt.Format(time.RFC3339))
	if plan.InfraTotalKLV > 0 {
		log.Printf("Infra cost: %.2f BRL @ %.6f BRL/KLV = %s KLV (total)",
			plan.InfraTotalBRL, plan.KLVBRLPrice, formatKLV(plan.InfraTotalKLV))
	} else {
		log.Printf("Infra cost: 0 (none configured)")
	}
	log.Printf("Balance floor (per validator): %s KLV", formatKLV(plan.BalanceFloor))

	var sumBalance, sumStaking, sumAllowance, sumInfra, sumWallets int64
	for _, vp := range plan.Validators {
		log.Printf("")
		log.Printf("[%s] owner %s", vp.Validator.DisplayName, vp.OwnerAddress)
		log.Printf("  balance:         %s KLV", formatKLV(vp.Balance))
		log.Printf("  staking rewards: %s KLV", formatKLV(vp.StakingRewards))
		log.Printf("  allowance:       %s KLV", formatKLV(vp.Allowance))
		log.Printf("  claimable total: %s KLV", formatKLV(vp.StakingRewards+vp.Allowance))
		sumBalance += vp.Balance
		sumStaking += vp.StakingRewards
		sumAllowance += vp.Allowance

		if vp.Skipped {
			log.Printf("  → SKIPPED: %s", vp.SkipReason)
			continue
		}
		log.Printf("  → distribute:    %s KLV (after floor %s)",
			formatKLV(vp.Balance+vp.StakingRewards+vp.Allowance-vp.BalanceFloor),
			formatKLV(vp.BalanceFloor))
		if vp.InfraShare > 0 {
			log.Printf("    infra share:   %s KLV → %s (%s)", formatKLV(vp.InfraShare), vp.InfraAddress, vp.InfraNickname)
			sumInfra += vp.InfraShare
		}
		for i, p := range vp.WalletPayouts {
			tag := ""
			if i == 0 && vp.DustToFirst > 0 {
				tag = fmt.Sprintf(" [+%s dust]", formatKLV(vp.DustToFirst))
			}
			log.Printf("    wallet share:  %s KLV → %s (%s)%s", formatKLV(p.Amount), p.Wallet.Address, p.Wallet.Nickname, tag)
			sumWallets += p.Amount
		}
	}

	log.Printf("")
	log.Printf("=== Totals ===")
	log.Printf("  balances:          %s KLV", formatKLV(sumBalance))
	log.Printf("  staking rewards:   %s KLV", formatKLV(sumStaking))
	log.Printf("  allowance:         %s KLV", formatKLV(sumAllowance))
	log.Printf("  claimable sum:     %s KLV", formatKLV(sumStaking+sumAllowance))
	log.Printf("  to infra manager:  %s KLV", formatKLV(sumInfra))
	log.Printf("  to validator wallets: %s KLV", formatKLV(sumWallets))
}

func (g *Generator) processValidator(vp *ValidatorPlan) error {
	contracts, contractTypes := g.buildContracts(vp)
	if len(contracts) == 0 {
		log.Printf("%s: nothing to do (no claims, no transfers)", vp.Validator.DisplayName)
		return nil
	}

	sendReq := map[string]any{
		"sender":    vp.OwnerAddress,
		"nonce":     vp.Nonce,
		"type":      contractTypes[0],
		"contracts": contracts,
	}
	if len(contracts) == 1 {
		sendReq["contract"] = contracts[0]
	}

	payload, err := json.Marshal(sendReq)
	if err != nil {
		return fmt.Errorf("marshal sendTXRequest: %w", err)
	}

	rawTxJSON, err := g.apiClient.BuildTransaction(payload)
	if err != nil {
		return fmt.Errorf("/transaction/send: %w", err)
	}

	var tx klproto.Transaction
	if err := json.Unmarshal(rawTxJSON, &tx); err != nil {
		return fmt.Errorf("decode proto.Transaction: %w (json=%s)", err, string(rawTxJSON))
	}
	if tx.RawData == nil {
		return fmt.Errorf("decoded transaction has no RawData (json=%s)", string(rawTxJSON))
	}

	rawBytes, err := g.marshalizer.Marshal(tx.RawData)
	if err != nil {
		return fmt.Errorf("marshal RawData: %w", err)
	}
	hash := g.hasher.Compute(string(rawBytes))
	hashHex := hex.EncodeToString(hash)

	if err := g.saveTxFile(vp, hashHex, rawTxJSON); err != nil {
		return fmt.Errorf("save tx file: %w", err)
	}

	if g.dryRun {
		log.Printf("%s: dry-run; tx hash %s saved locally only", vp.Validator.DisplayName, hashHex)
		return nil
	}

	if err := g.postToMultisig(vp.OwnerAddress, hashHex, rawTxJSON); err != nil {
		return fmt.Errorf("post to multisig: %w", err)
	}
	log.Printf("%s: posted multisig transaction %s", vp.Validator.DisplayName, hashHex)
	return nil
}

func (g *Generator) buildContracts(vp *ValidatorPlan) ([]map[string]any, []uint32) {
	contracts := []map[string]any{}
	types := []uint32{}

	if vp.StakingRewards > 0 {
		contracts = append(contracts, map[string]any{
			"contractType": contractTypeClaim,
			"claimType":    claimTypeStaking,
			"id":           assetKLV,
		})
		types = append(types, contractTypeClaim)
	}
	if vp.Allowance > 0 {
		contracts = append(contracts, map[string]any{
			"contractType": contractTypeClaim,
			"claimType":    claimTypeAllowance,
			"id":           assetKLV,
		})
		types = append(types, contractTypeClaim)
	}
	if vp.InfraShare > 0 && vp.InfraAddress != "" {
		contracts = append(contracts, map[string]any{
			"contractType": contractTypeTransfer,
			"receiver":     vp.InfraAddress,
			"amount":       vp.InfraShare,
			"kda":          assetKLV,
		})
		types = append(types, contractTypeTransfer)
	}
	for _, p := range vp.WalletPayouts {
		contracts = append(contracts, map[string]any{
			"contractType": contractTypeTransfer,
			"receiver":     p.Wallet.Address,
			"amount":       p.Amount,
			"kda":          assetKLV,
		})
		types = append(types, contractTypeTransfer)
	}
	return contracts, types
}

func (g *Generator) saveTxFile(vp *ValidatorPlan, hashHex string, rawTxJSON json.RawMessage) error {
	safeName := sanitizeFilename(vp.Validator.DisplayName)
	path := filepath.Join(g.outputDir, fmt.Sprintf("%s.json", safeName))
	body := map[string]any{
		"validator":    vp.Validator.DisplayName,
		"ownerAddress": vp.OwnerAddress,
		"hash":         hashHex,
		"raw":          rawTxJSON,
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (g *Generator) postToMultisig(owner, hashHex string, rawTxJSON json.RawMessage) error {
	body := map[string]any{
		"Hash":    hashHex,
		"Address": owner,
		"Raw":     rawTxJSON,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/transaction?network=KLV", g.cfg.Payouts.MultisigAPIURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("multisig api returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func formatKLV(atomic int64) string {
	sign := ""
	if atomic < 0 {
		sign = "-"
		atomic = -atomic
	}
	whole := atomic / klvAtomicUnitsPerUnit
	frac := atomic % klvAtomicUnitsPerUnit
	return fmt.Sprintf("%s%d.%0*d", sign, whole, klvPrecision, frac)
}

func sanitizeFilename(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_' || r == '.':
			out = append(out, r)
		case r == ' ':
			out = append(out, '_')
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "validator"
	}
	return string(out)
}
