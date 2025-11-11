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
	NodeBaseURL     string                      `yaml:"nodeBaseUrl"`
	APIBaseURL     string                       `yaml:"apiBaseUrl"`
	PollInterval   int                          `yaml:"pollInterval"`
	Validators     []models.MonitoredValidator  `yaml:"validators"`
	Slack          SlackConfig                  `yaml:"slack"`
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

	return &cfg, nil
}
