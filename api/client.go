package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
