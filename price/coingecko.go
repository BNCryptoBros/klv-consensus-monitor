package price

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) FetchKLVPerBRL(apiURL, jsonPath string) (float64, error) {
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return 0, fmt.Errorf("fetch price: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read price: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("price api returned %d: %s", resp.StatusCode, string(body))
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, fmt.Errorf("parse price: %w", err)
	}

	value, err := walkPath(raw, jsonPath)
	if err != nil {
		return 0, err
	}

	priceBRLPerKLV, err := toFloat(value)
	if err != nil {
		return 0, fmt.Errorf("price at %q is not numeric: %w", jsonPath, err)
	}
	if priceBRLPerKLV <= 0 {
		return 0, fmt.Errorf("price at %q is non-positive: %v", jsonPath, value)
	}
	return priceBRLPerKLV, nil
}

func walkPath(node any, path string) (any, error) {
	if path == "" {
		return node, nil
	}
	parts := strings.Split(path, ".")
	current := node
	for i, p := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q: segment %q (#%d) does not address an object", path, p, i)
		}
		next, exists := m[p]
		if !exists {
			return nil, fmt.Errorf("path %q: key %q not found at segment #%d", path, p, i)
		}
		current = next
	}
	return current, nil
}

func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int64:
		return float64(x), nil
	case int:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	}
	return 0, fmt.Errorf("unsupported numeric type %T", v)
}
