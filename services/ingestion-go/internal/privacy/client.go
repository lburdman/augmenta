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
	llmGatewayURL string
	httpClient    *http.Client
}

func NewClient(privacyURL, llmGatewayURL string) *Client {
	return &Client{
		privacyURL:    privacyURL,
		llmGatewayURL: llmGatewayURL,
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

// CompleteLLM forwards the anonymized payload to the LLM Gateway.
func (c *Client) CompleteLLM(ctx context.Context, req types.LLMGatewayRequest) (*types.LLMGatewayResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal llm gateway request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.llmGatewayURL+"/complete", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create llm gateway request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send llm gateway request: %w", err)
	}
	defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm gateway returned status: %d", resp.StatusCode)
	}

	var gatewayResp types.LLMGatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&gatewayResp); err != nil {
		return nil, fmt.Errorf("failed to decode llm gateway response: %w", err)
	}

	return &gatewayResp, nil
}
