package slack

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Notifier struct {
	webhookURL      string
	messageTemplate string
	httpClient      *http.Client
	enabled         bool
}

func NewNotifier(enabled bool, webhookURL, messageTemplate string) *Notifier {
	return &Notifier{
		enabled:         enabled,
		webhookURL:      webhookURL,
		messageTemplate: messageTemplate,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *Notifier) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error {
	if !n.enabled {
		return nil
	}

	if n.webhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}

	message := n.buildMessage(displayName, oldStatus, newStatus, epoch)

	if err := n.post(message); err != nil {
		return err
	}

	log.Printf("Slack notification sent for %s status change", displayName)
	return nil
}

func (n *Notifier) Enabled() bool {
	return n.enabled
}

func (n *Notifier) PostMessage(payload string) error {
	if !n.enabled {
		return nil
	}
	if n.webhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}
	return n.post(payload)
}

func (n *Notifier) post(payload string) error {
	resp, err := n.httpClient.Post(n.webhookURL, "application/json", bytes.NewBufferString(payload))
	if err != nil {
		return fmt.Errorf("failed to send slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status: %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) buildMessage(displayName, oldStatus, newStatus string, epoch int) string {
	message := n.messageTemplate

	replacements := map[string]string{
		"{{displayName}}": displayName,
		"{{oldStatus}}":   oldStatus,
		"{{newStatus}}":   newStatus,
		"{{epoch}}":       strconv.Itoa(epoch),
	}

	for placeholder, value := range replacements {
		message = strings.ReplaceAll(message, placeholder, value)
	}

	return message
}
