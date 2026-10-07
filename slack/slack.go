package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/notify"
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

func (n *Notifier) Name() string {
	return "Slack"
}

func (n *Notifier) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error {
	if !n.enabled {
		return nil
	}

	if n.webhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}

	message := notify.RenderStatus(n.messageTemplate, displayName, oldStatus, newStatus, epoch, func(s string) string { return s })

	if err := n.post(message); err != nil {
		return err
	}

	log.Printf("Slack notification sent for %s status change", displayName)
	return nil
}

func (n *Notifier) SendPayday(summary notify.Summary) error {
	if !n.enabled {
		return nil
	}

	var totalsBuf strings.Builder
	totalsBuf.WriteString("*How much each wallet pockets today:*\n")
	for _, t := range summary.Totals {
		fmt.Fprintf(&totalsBuf, "  • *%s* (`%s`): *%s KLV*\n",
			t.Nickname, t.Address, notify.FormatKLV(t.Amount))
	}
	fmt.Fprintf(&totalsBuf, "\n_Grand total being moved:_ *%s KLV*", notify.FormatKLV(summary.GrandTotal))

	header := notify.PaydayHeader
	subtitle := fmt.Sprintf(notify.PaydaySubtitle, summary.Submitted)

	payload := map[string]any{
		"text": header,
		"blocks": []any{
			map[string]any{
				"type": "header",
				"text": map[string]any{"type": "plain_text", "text": header, "emoji": true},
			},
			map[string]any{
				"type": "section",
				"text": map[string]any{"type": "mrkdwn", "text": subtitle},
			},
			map[string]any{"type": "divider"},
			map[string]any{
				"type": "section",
				"text": map[string]any{"type": "mrkdwn", "text": totalsBuf.String()},
			},
			map[string]any{
				"type": "context",
				"elements": []any{
					map[string]any{
						"type": "mrkdwn",
						"text": fmt.Sprintf("Sign the transactions at <%s|kleverscan.org/multisign>", notify.MultisignURL),
					},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return n.postMessage(string(raw))
}

func (n *Notifier) SendFailure(runErr error, submitted int) error {
	if !n.enabled {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"text": "🚨 " + notify.FailureText(runErr, submitted)})
	if err != nil {
		return err
	}
	return n.postMessage(string(raw))
}

func (n *Notifier) postMessage(payload string) error {
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
