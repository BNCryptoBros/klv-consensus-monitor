package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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

func (n *Notifier) SendStatusChange(displayName, oldStatus, newStatus string, epoch int) error {
	if !n.enabled {
		return nil
	}

	if err := n.validate(); err != nil {
		return err
	}

	message := n.buildMessage(displayName, oldStatus, newStatus, epoch)

	if err := n.post(message); err != nil {
		return err
	}

	log.Printf("Telegram notification sent for %s status change", displayName)
	return nil
}

func (n *Notifier) Enabled() bool {
	return n.enabled
}

func (n *Notifier) PostMessage(text string) error {
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

func (n *Notifier) buildMessage(displayName, oldStatus, newStatus string, epoch int) string {
	message := n.messageTemplate

	replacements := map[string]string{
		"{{displayName}}": html.EscapeString(displayName),
		"{{oldStatus}}":   html.EscapeString(oldStatus),
		"{{newStatus}}":   html.EscapeString(newStatus),
		"{{epoch}}":       strconv.Itoa(epoch),
	}

	for placeholder, value := range replacements {
		message = strings.ReplaceAll(message, placeholder, value)
	}

	return message
}

func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "<redacted>"))
}
