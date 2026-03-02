package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	ingestionURL  = "http://localhost:8080/ingest/webhook/demo"
	gatewayURL = "http://llm-gateway-go:7000/last"
)

func TestIngestionForwardingWithoutPII(t *testing.T) {
	// Start with a brief wait to ensure services are fully up (if run directly after docker-compose up)
	time.Sleep(2 * time.Second)

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Send the ingestion payload containing an email
	payload := map[string]string{
		"text": "Hello, my email is john.doe@example.com, please contact me.",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, ingestionURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	// Tenant configured in flows.yaml
	req.Header.Set("X-Tenant-ID", "tenantA")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send ingestion request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from ingestion, got %d", resp.StatusCode)
	}

	var ingestResp struct {
		RequestID      string `json:"requestId"`
		AnonymizedText string `json:"anonymized_text"`
		LLMOutput      string `json:"llm_output"`
		Provider       string `json:"provider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		t.Fatalf("Failed to decode ingestion response: %v", err)
	}

	if ingestResp.RequestID == "" {
		t.Error("Ingestion response missing RequestID")
	}
	if !strings.Contains(ingestResp.AnonymizedText, "<REDACTED>") {
		t.Errorf("Ingestion response text not redacted: %s", ingestResp.AnonymizedText)
	}
	if !strings.HasPrefix(ingestResp.LLMOutput, "ECHO: ") {
		t.Errorf("Ingestion LLMOutput does not start with ECHO: %s", ingestResp.LLMOutput)
	}
	if ingestResp.Provider != "echo" {
		t.Errorf("Expected provider 'echo', got %s", ingestResp.Provider)
	}

	// 2. Fetch the last recieved payload from the gateway mock
	resp2, err := client.Get(gatewayURL)
	if err != nil {
		t.Fatalf("Failed to fetch from gateway: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from gateway, got %d", resp2.StatusCode)
	}

	var gatewayResp map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&gatewayResp); err != nil {
		t.Fatalf("Failed to decode gateway response: %v", err)
	}

	// 3 & 4. Assertions on the downstream payload
	reqIDDownstream, ok := gatewayResp["requestId"].(string)
	if !ok || reqIDDownstream != ingestResp.RequestID {
		t.Errorf("Gateway RequestID Mismatch. Expected=%s, Got=%v", ingestResp.RequestID, reqIDDownstream)
	}

	// Double check that we aren't leaking the email in any field of the dictionary metadata
	rawJSON, _ := json.Marshal(gatewayResp)
	if strings.Contains(string(rawJSON), "john.doe@example.com") {
		t.Errorf("CRITICAL FAILURE: LLM Gateway payload contained raw PII email. Text received: %s", string(rawJSON))
	}
}
