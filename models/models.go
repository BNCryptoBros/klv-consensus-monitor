package models

type MonitoredValidator struct {
	BLSKey       string `yaml:"blsKey"`
	DisplayName  string `yaml:"displayName"`
	OwnerAddress string `yaml:"ownerAddress,omitempty"`
}

type ValidatorInfo struct {
	BLSPublicKey	string `json:"-"`
	ValidatorStatus string `json:"ValidatorStatus"`
}

type BlockInfo struct {
	Epoch  int    `json:"epoch"`
}

type ValidatorStatisticsResponse struct {
	Data struct {
		Statistics map[string]ValidatorInfo `json:"statistics"`
	} `json:"data"`
}

type BlockListResponse struct {
	Data struct {
		Blocks []BlockInfo `json:"blocks"`
	} `json:"data"`
}

type ValidatorState struct {
	BLSKey      string
	DisplayName string
	Status      string
	Epoch       int
}

type ValidatorWallet struct {
	Address  string `yaml:"address"`
	Nickname string `yaml:"nickname"`
}

type InfraCostConfig struct {
	AmountBRLCents  int64  `yaml:"amountBRLCents"`
	PriceAPIURL     string `yaml:"priceApiUrl"`
	PriceJSONPath   string `yaml:"priceJsonPath"`
	ManagerAddress  string `yaml:"managerAddress"`
	ManagerNickname string `yaml:"managerNickname"`
}

type PayoutsConfig struct {
	MultisigAPIURL   string            `yaml:"multisigApiUrl"`
	ValidatorWallets []ValidatorWallet `yaml:"validatorWallets"`
	BalanceFloor     int64             `yaml:"balanceFloor"`
	InfraCost        InfraCostConfig   `yaml:"infraCost"`
}

type ValidatorAPIInfo struct {
	OwnerAddress string `json:"ownerAddress"`
	BLSPublicKey string `json:"blsPublicKey"`
	Name         string `json:"name"`
}

type ValidatorListResponse struct {
	Data struct {
		Validators []ValidatorAPIInfo `json:"validators"`
	} `json:"data"`
	Pagination struct {
		Self     int `json:"self"`
		Next     int `json:"next"`
		Previous int `json:"previous"`
		PerPage  int `json:"perPage"`
		Total    int `json:"totalPages"`
	} `json:"pagination"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

type AccountInfoResponse struct {
	Data struct {
		Account struct {
			Address string `json:"Address"`
			Balance int64  `json:"Balance"`
			Nonce   uint64 `json:"Nonce"`
		} `json:"account"`
	} `json:"data"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

type AvailableClaimResponse struct {
	Data struct {
		StakingRewards int64            `json:"stakingRewards"`
		Allowance      int64            `json:"allowance"`
		AllStaking     map[string]int64 `json:"allStakingRewards"`
	} `json:"data"`
	Error string `json:"error"`
	Code  string `json:"code"`
}
