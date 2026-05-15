package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/api"
	"github.com/BNCryptoBros/klv-consensus-monitor/config"
	"github.com/BNCryptoBros/klv-consensus-monitor/monitor"
	"github.com/BNCryptoBros/klv-consensus-monitor/payments"
	"github.com/BNCryptoBros/klv-consensus-monitor/slack"
)

func main() {
	log.SetFlags(0)

	mode := flag.String("mode", "monitor", "run mode: monitor | payments")
	configPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "payments mode: build txs locally but do not POST to multisig API")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if len(cfg.Validators) == 0 {
		log.Fatal("No validators configured in config.yaml")
	}

	log.Printf("API Base URL: %s", cfg.APIBaseURL)
	log.Printf("Node Base URL: %s", cfg.NodeBaseURL)

	apiClient := api.NewClient(cfg.APIBaseURL, cfg.NodeBaseURL)

	switch *mode {
	case "monitor":
		runMonitor(cfg, apiClient)
	case "payments":
		runPayments(cfg, apiClient, *dryRun)
	default:
		log.Fatalf("Unknown mode %q (expected: monitor | payments)", *mode)
	}
}

func runMonitor(cfg *config.Config, apiClient *api.Client) {
	log.Printf("Loaded configuration: monitoring %d validators", len(cfg.Validators))
	log.Printf("Poll Interval: %d seconds", cfg.PollInterval)
	log.Printf("Slack notifications: %v", cfg.Slack.Enabled)

	slackNotifier := slack.NewNotifier(cfg.Slack.Enabled, cfg.Slack.WebhookURL, cfg.Slack.MessageTemplate)
	mon := monitor.NewMonitor(apiClient, slackNotifier, cfg.Validators)

	if err := mon.Run(); err != nil {
		log.Fatalf("Failed to initialize monitor: %v", err)
	}

	ticker := time.NewTicker(time.Duration(cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Monitor started successfully. Press Ctrl+C to stop.")

	for {
		select {
		case <-ticker.C:
			if err := mon.Check(); err != nil {
				log.Printf("Error during check: %v", err)
			}
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			return
		}
	}
}

func runPayments(cfg *config.Config, apiClient *api.Client, dryRun bool) {
	if dryRun {
		log.Printf("Running payments mode (DRY RUN — no multisig submission)")
	} else {
		log.Printf("Running payments mode — will POST unsigned transactions to %s", cfg.Payouts.MultisigAPIURL)
	}
	gen := payments.NewGenerator(cfg, apiClient, dryRun)
	if err := gen.Run(); err != nil {
		log.Fatalf("payments run failed: %v", err)
	}
}
