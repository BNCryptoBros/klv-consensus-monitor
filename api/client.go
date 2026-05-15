package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/BNCryptoBros/klv-consensus-monitor/models"
)

type Client struct {
	baseAPIURL    string
	baseNodeURL    string
	httpClient *http.Client
}

func NewClient(baseAPIURL, baseNodeURL string) *Client {
	return &Client{
		baseAPIURL: baseAPIURL,
		baseNodeURL: baseNodeURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) FetchValidatorList() ([]models.ValidatorInfo, error) {
	url := fmt.Sprintf("%s/validator/statistics", c.baseNodeURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch validator statistics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response models.ValidatorStatisticsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse validator statistics: %w", err)
	}

	log.Printf("Validator statistics response: %d validators", len(response.Data.Statistics))

	validators := make([]models.ValidatorInfo, 0, len(response.Data.Statistics))
	for blsKey, validator := range response.Data.Statistics {
		validator.BLSPublicKey = blsKey
		validators = append(validators, validator)
	}

	return validators, nil
}

func (c *Client) FetchValidatorByBLS(blsKey string) (*models.ValidatorAPIInfo, error) {
	page := 1
	const perPage = 100
	for {
		listURL := fmt.Sprintf("%s/v1.0/validator/list?page=%d&limit=%d", c.baseAPIURL, page, perPage)
		resp, err := c.httpClient.Get(listURL)
		if err != nil {
			return nil, fmt.Errorf("fetch validator list page %d: %w", page, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read validator list page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("validator list page %d returned %d: %s", page, resp.StatusCode, string(body))
		}

		var listResp models.ValidatorListResponse
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, fmt.Errorf("parse validator list page %d: %w", page, err)
		}

		for i := range listResp.Data.Validators {
			v := &listResp.Data.Validators[i]
			if v.BLSPublicKey == blsKey {
				return v, nil
			}
		}

		if len(listResp.Data.Validators) < perPage {
			return nil, fmt.Errorf("validator with bls key %s not found in any page", blsKey)
		}
		page++
		if page > 200 {
			return nil, fmt.Errorf("validator with bls key %s not found within 200 pages", blsKey)
		}
	}
}

func (c *Client) FetchAccountInfo(address string) (*models.AccountInfoResponse, error) {
	endpoint := fmt.Sprintf("%s/address/%s", c.baseNodeURL, url.PathEscape(address))
	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch account %s: %w", address, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read account %s: %w", address, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account %s returned %d: %s", address, resp.StatusCode, string(body))
	}
	var out models.AccountInfoResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse account %s: %w", address, err)
	}
	return &out, nil
}

func (c *Client) FetchAvailableClaim(address, asset string) (*models.AvailableClaimResponse, error) {
	endpoint := fmt.Sprintf("%s/address/%s/allowance?asset=%s", c.baseNodeURL, url.PathEscape(address), url.QueryEscape(asset))
	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch allowance %s: %w", address, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read allowance %s: %w", address, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("allowance %s returned %d: %s", address, resp.StatusCode, string(body))
	}
	var out models.AvailableClaimResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse allowance %s: %w", address, err)
	}
	return &out, nil
}

func (c *Client) BuildTransaction(payload []byte) (json.RawMessage, error) {
	endpoint := fmt.Sprintf("%s/transaction/send", c.baseNodeURL)
	resp, err := c.httpClient.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("post /transaction/send: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read /transaction/send: %w", err)
	}

	var envelope struct {
		Data struct {
			Result json.RawMessage `json:"result"`
			TxHash string          `json:"txHash"`
		} `json:"data"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse /transaction/send: %w (body=%s)", err, string(body))
	}
	if envelope.Error != "" {
		return nil, fmt.Errorf("/transaction/send error: %s", envelope.Error)
	}
	if len(envelope.Data.Result) == 0 || string(envelope.Data.Result) == "null" {
		return nil, fmt.Errorf("/transaction/send returned no result (status=%d body=%s)", resp.StatusCode, string(body))
	}
	return envelope.Data.Result, nil
}

func (c *Client) FetchLatestBlock() (*models.BlockInfo, error) {
	url := fmt.Sprintf("%s/v1.0/block/list?page=1&limit=1", c.baseAPIURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest block: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response models.BlockListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse block list: %w", err)
	}

	if len(response.Data.Blocks) == 0 {
		return nil, fmt.Errorf("no blocks in response")
	}

	return &response.Data.Blocks[0], nil
}
