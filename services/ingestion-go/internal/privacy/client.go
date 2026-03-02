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
	privacyClient *http.Client
	llmClient     *http.Client
}

func NewClient(privacyURL, llmGatewayURL string, privacyTimeout, llmTimeout time.Duration) *Client {
	return &Client{
		privacyURL:    privacyURL,
		llmGatewayURL: llmGatewayURL,
		privacyClient: &http.Client{
			Timeout: privacyTimeout,
		},
		llmClient: &http.Client{
			Timeout: llmTimeout,
		},
	}
}

// doWithRetry executes an HTTP request with exactly 1 retry (100ms backoff) on network/timeout errors.
func doWithRetry(ctx context.Context, client *http.Client, reqMaker func(ctx context.Context) (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := reqMaker(ctx)
		if err != nil {
			return nil, err
		}
		
		resp, err := client.Do(req)
		if err == nil {
			// Success mapping
			return resp, nil
		}

		lastErr = err
		// Do not retry if context is cancelled or timeout exceeded
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Small backoff before retry (only if attempt 0)
		if attempt == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil, lastErr
}

// Anonymize calls the privacy service to redact the text based on configured operators.
func (c *Client) Anonymize(ctx context.Context, req types.PrivacyAnonymizeRequest) (*types.PrivacyAnonymizeResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anonymize request: %w", err)
	}

	reqMaker := func(innerCtx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(innerCtx, http.MethodPost, c.privacyURL+"/anonymize", bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create anonymize request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		return httpReq, nil
	}

	resp, err := doWithRetry(ctx, c.privacyClient, reqMaker)
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

	reqMaker := func(innerCtx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(innerCtx, http.MethodPost, c.llmGatewayURL+"/complete", bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create llm gateway request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		return httpReq, nil
	}

	resp, err := doWithRetry(ctx, c.llmClient, reqMaker)
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
