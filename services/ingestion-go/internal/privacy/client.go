package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

type Client struct {
	privacyURL    string
	downstreamURL string
	httpClient    *http.Client
}

func NewClient(privacyURL, downstreamURL string) *Client {
	return &Client{
		privacyURL:    privacyURL,
		downstreamURL: downstreamURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Anonymize calls the privacy service to redact the text based on configured operators.
func (c *Client) Anonymize(ctx context.Context, req types.PrivacyAnonymizeRequest) (*types.PrivacyAnonymizeResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anonymize request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.privacyURL+"/anonymize", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create anonymize request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send anonymize request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("privacy service returned status: %d", resp.StatusCode)
	}

	var providerResp types.PrivacyAnonymizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&providerResp); err != nil {
		return nil, fmt.Errorf("failed to decode privacy response: %w", err)
	}

	return &providerResp, nil
}

// ForwardDownstream forwards the anonymized payload to the mock downstream service.
func (c *Client) ForwardDownstream(ctx context.Context, req types.DownstreamReceiveRequest) (int, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal downstream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.downstreamURL+"/receive", bytes.NewReader(reqBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create downstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("failed to send downstream request: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}
