package config

import (
	"fmt"
	"os"

	"github.com/BNCryptoBros/klv-consensus-monitor/models"
	"gopkg.in/yaml.v3"
)

type SlackConfig struct {
	Enabled         bool   `yaml:"enabled"`
	WebhookURL      string `yaml:"webhookUrl"`
	MessageTemplate string `yaml:"messageTemplate"`
}

type Config struct {
	NodeBaseURL    string                      `yaml:"nodeBaseUrl"`
	APIBaseURL     string                      `yaml:"apiBaseUrl"`
	PollInterval   int                         `yaml:"pollInterval"`
	Validators     []models.MonitoredValidator `yaml:"validators"`
	Slack          SlackConfig                 `yaml:"slack"`
	Payouts        models.PayoutsConfig        `yaml:"payouts"`
}

func Load(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.mainnet.klever.org"
	}

	if cfg.NodeBaseURL == "" {
		cfg.NodeBaseURL = "https://node.mainnet.klever.org"
	}

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30
	}

	if cfg.Payouts.MultisigAPIURL == "" {
		cfg.Payouts.MultisigAPIURL = "https://multisign.mainnet.klever.org"
	}

	if cfg.Payouts.InfraCost.PriceAPIURL == "" {
		cfg.Payouts.InfraCost.PriceAPIURL = "https://api.coingecko.com/api/v3/simple/price?ids=klever&vs_currencies=brl"
	}

	if cfg.Payouts.InfraCost.PriceJSONPath == "" {
		cfg.Payouts.InfraCost.PriceJSONPath = "klever.brl"
	}

	return &cfg, nil
}

func (c *Config) ValidatePayouts() error {
	if len(c.Payouts.ValidatorWallets) == 0 {
		return fmt.Errorf("payouts.validatorWallets must contain at least one entry")
	}
	for i, w := range c.Payouts.ValidatorWallets {
		if w.Address == "" {
			return fmt.Errorf("payouts.validatorWallets[%d].address is required", i)
		}
		if w.Nickname == "" {
			return fmt.Errorf("payouts.validatorWallets[%d].nickname is required", i)
		}
	}
	if c.Payouts.BalanceFloor < 0 {
		return fmt.Errorf("payouts.balanceFloor must be >= 0")
	}
	if c.Payouts.InfraCost.AmountBRLCents < 0 {
		return fmt.Errorf("payouts.infraCost.amountBRLCents must be >= 0")
	}
	if c.Payouts.InfraCost.ManagerAddress == "" {
		return fmt.Errorf("payouts.infraCost.managerAddress is required")
	}
	return nil
}
