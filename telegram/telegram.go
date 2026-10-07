package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/notify"
)

const apiBaseURL = "https://api.telegram.org"

type Notifier struct {
	botToken        string
	chatID          string
	messageTemplate string
	httpClient      *http.Client
	enabled         bool
}

func NewNotifier(enabled bool, botToken, chatID, messageTemplate string) *Notifier {
	return &Notifier{
		enabled:         enabled,
		botToken:        botToken,
		chatID:          chatID,
		messageTemplate: messageTemplate,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *Notifier) Name() string {
	return "Telegram"
}

func (n *Notifier) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error {
	if !n.enabled {
		return nil
	}

	if err := n.validate(); err != nil {
		return err
	}

	message := notify.RenderStatus(n.messageTemplate, displayName, oldStatus, newStatus, epoch, html.EscapeString)

	if err := n.post(message); err != nil {
		return err
	}

	log.Printf("Telegram notification sent for %s status change", displayName)
	return nil
}

func (n *Notifier) SendPayday(summary notify.Summary) error {
	if !n.enabled {
		return nil
	}

	var msg strings.Builder
	fmt.Fprintf(&msg, "<b>%s</b>\n\n", html.EscapeString(notify.PaydayHeader))
	fmt.Fprintf(&msg, "%s\n\n", html.EscapeString(fmt.Sprintf(notify.PaydaySubtitle, summary.Submitted)))
	msg.WriteString("<b>How much each wallet pockets today:</b>\n")
	for _, t := range summary.Totals {
		fmt.Fprintf(&msg, "  • <b>%s</b> (<code>%s</code>): <b>%s KLV</b>\n",
			html.EscapeString(t.Nickname), html.EscapeString(t.Address), notify.FormatKLV(t.Amount))
	}
	fmt.Fprintf(&msg, "\n<i>Grand total being moved:</i> <b>%s KLV</b>\n\n", notify.FormatKLV(summary.GrandTotal))
	fmt.Fprintf(&msg, "Sign the transactions at <a href=\"%s\">kleverscan.org/multisign</a>", notify.MultisignURL)

	return n.postMessage(msg.String())
}

func (n *Notifier) SendFailure(runErr error, submitted int) error {
	return n.postMessage("🚨 " + html.EscapeString(notify.FailureText(runErr, submitted)))
}

func (n *Notifier) postMessage(text string) error {
	if !n.enabled {
		return nil
	}
	if err := n.validate(); err != nil {
		return err
	}
	return n.post(text)
}

func (n *Notifier) validate() error {
	if n.botToken == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	if n.chatID == "" {
		return fmt.Errorf("telegram chat ID is not configured")
	}
	return nil
}

func (n *Notifier) post(text string) error {
	payload, err := json.Marshal(map[string]any{
		"chat_id":                  n.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil {
		return fmt.Errorf("failed to encode telegram message: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", apiBaseURL, n.botToken)
	resp, err := n.httpClient.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to send telegram notification: %w", redactToken(err, n.botToken))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "<redacted>"))
}
