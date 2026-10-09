package telegram

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestTransportErrorDoesNotExposeBotToken(t *testing.T) {
	const token = "123456:ABC-secret-token"

	tests := []struct {
		name string
		send func(n *Notifier) error
	}{
		{
			name: "status change",
			send: func(n *Notifier) error {
				return n.SendStatusChange("validator", "eligible", "jailed", 42)
			},
		},
		{
			name: "failure",
			send: func(n *Notifier) error {
				return n.SendFailure(errors.New("payments run failed"), 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestedURL string
			n := NewNotifier(true, token, "-1001234567890", "{name}: {old} -> {new}")
			n.httpClient = &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					requestedURL = r.URL.String()
					return nil, errors.New("dial tcp: connection refused")
				}),
			}

			err := tt.send(n)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(requestedURL, token) {
				t.Fatalf("expected request URL to carry the token, got %q", requestedURL)
			}
			if strings.Contains(err.Error(), token) {
				t.Fatalf("expected error without bot token, got %q", err.Error())
			}
			if !strings.Contains(err.Error(), "<redacted>") {
				t.Fatalf("expected error to contain redaction marker, got %q", err.Error())
			}
		})
	}
}

func TestRedactToken(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		token string
		want  string
	}{
		{
			name:  "empty token returns error unchanged",
			err:   errors.New("Post https://api.telegram.org/bot/sendMessage: timeout"),
			token: "",
			want:  "Post https://api.telegram.org/bot/sendMessage: timeout",
		},
		{
			name:  "every token occurrence is replaced",
			err:   errors.New("Post https://api.telegram.org/bot123456:ABC-secret-token/sendMessage: 123456:ABC-secret-token"),
			token: "123456:ABC-secret-token",
			want:  "Post https://api.telegram.org/bot<redacted>/sendMessage: <redacted>",
		},
		{
			name:  "error without token is unchanged",
			err:   errors.New("connection refused"),
			token: "123456:ABC-secret-token",
			want:  "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactToken(tt.err, tt.token)
			if got.Error() != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got.Error())
			}
		})
	}
}
