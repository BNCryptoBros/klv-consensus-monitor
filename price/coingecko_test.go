package price

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchRetryBehavior(t *testing.T) {
	retryDelays := []time.Duration{0, 0}
	exhausted := int32(len(retryDelays) + 1)

	tests := []struct {
		name      string
		handler   func(hit int32, w http.ResponseWriter, r *http.Request)
		wantHits  int32
		wantPrice float64
		checkErr  func(t *testing.T, err error)
	}{
		{
			name: "timeout is retried and surfaces a timeout error",
			handler: func(hit int32, w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(200 * time.Millisecond):
				}
			},
			wantHits: exhausted,
			checkErr: func(t *testing.T, err error) {
				var netErr net.Error
				if !errors.As(err, &netErr) || !netErr.Timeout() {
					t.Fatalf("expected timeout error, got %v", err)
				}
			},
		},
		{
			name: "429 is retried until attempts are exhausted",
			handler: func(hit int32, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantHits: exhausted,
			checkErr: func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), "429") {
					t.Fatalf("expected error mentioning 429, got %v", err)
				}
			},
		},
		{
			name: "500 is retried until attempts are exhausted",
			handler: func(hit int32, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantHits: exhausted,
			checkErr: func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), "500") {
					t.Fatalf("expected error mentioning 500, got %v", err)
				}
			},
		},
		{
			name: "400 is not retried",
			handler: func(hit int32, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			wantHits: 1,
			checkErr: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "500 then 200 returns the price",
			handler: func(hit int32, w http.ResponseWriter, r *http.Request) {
				if hit == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"klever":{"brl":0.0123}}`))
			},
			wantHits:  2,
			wantPrice: 0.0123,
			checkErr: func(t *testing.T, err error) {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.handler(hits.Add(1), w, r)
			}))
			defer server.Close()

			client := &Client{
				httpClient:  &http.Client{Timeout: 50 * time.Millisecond},
				retryDelays: retryDelays,
			}

			price, err := client.FetchKLVPerBRL(server.URL, "klever.brl")
			tt.checkErr(t, err)
			if got := hits.Load(); got != tt.wantHits {
				t.Fatalf("expected %d hits, got %d", tt.wantHits, got)
			}
			if price != tt.wantPrice {
				t.Fatalf("expected price %v, got %v", tt.wantPrice, price)
			}
		})
	}
}
