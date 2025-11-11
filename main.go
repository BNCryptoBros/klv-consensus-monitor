package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/api"
	"github.com/BNCryptoBros/klv-consensus-monitor/config"
	"github.com/BNCryptoBros/klv-consensus-monitor/monitor"
	"github.com/BNCryptoBros/klv-consensus-monitor/slack"
)

func main() {
	log.SetFlags(0)

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if len(cfg.Validators) == 0 {
		log.Fatal("No validators configured in config.yaml")
	}

	log.Printf("Loaded configuration: monitoring %d validators", len(cfg.Validators))
	log.Printf("API Base URL: %s", cfg.APIBaseURL)
	log.Printf("Node Base URL: %s", cfg.NodeBaseURL)
	log.Printf("Poll Interval: %d seconds", cfg.PollInterval)
	log.Printf("Slack notifications: %v", cfg.Slack.Enabled)

	apiClient := api.NewClient(cfg.APIBaseURL, cfg.NodeBaseURL)
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
