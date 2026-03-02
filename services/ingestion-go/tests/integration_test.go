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
	downstreamURL = "http://localhost:9000/last"
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
		RequestID        string `json:"requestId"`
		AnonymizedText   string `json:"anonymized_text"`
		DownstreamStatus int    `json:"downstream_status"`
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
	if ingestResp.DownstreamStatus != 200 {
		t.Errorf("Expected downstream status 200, got %d", ingestResp.DownstreamStatus)
	}

	// 2. Fetch the last recieved payload from the downstream mock
	resp2, err := client.Get(downstreamURL)
	if err != nil {
		t.Fatalf("Failed to fetch from downstream mock: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from downstream mock, got %d", resp2.StatusCode)
	}

	var downstreamResp map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&downstreamResp); err != nil {
		t.Fatalf("Failed to decode downstream response: %v", err)
	}

	// 3 & 4. Assertions on the downstream payload
	reqIDDownstream, ok := downstreamResp["requestId"].(string)
	if !ok || reqIDDownstream != ingestResp.RequestID {
		t.Errorf("Downstream RequestID Mismatch. Expected=%s, Got=%v", ingestResp.RequestID, reqIDDownstream)
	}

	downstreamText, ok := downstreamResp["anonymized_text"].(string)
	if !ok || downstreamText == "" {
		t.Errorf("Downstream payload missing anonymized_text")
	}

	if strings.Contains(downstreamText, "john.doe@example.com") {
		t.Errorf("CRITICAL FAILURE: Downstream payload contained raw PII email. Text received: %s", downstreamText)
	}
	
	if !strings.Contains(downstreamText, "<REDACTED>") {
		t.Errorf("Downstream text missing REDACTED token. Text received: %s", downstreamText)
	}
}
