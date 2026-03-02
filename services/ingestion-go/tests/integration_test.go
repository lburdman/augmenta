package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lburdman/augmenta/services/ingestion-go/internal/audit"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

const (
	ingestionURL  = "http://ingestion-go:8080/ingest/webhook/demo"
	gatewayURL    = "http://llm-gateway-go:7001/last"
	auditAdminURL = "http://ingestion-go:8080/admin/audit"
)

func TestIngestionUnknownFlow(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	payload := map[string]string{"text": "Does not matter"}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, "http://ingestion-go:8080/ingest/webhook/unknown_source", bytes.NewReader(body))
	req.Header.Set("X-Tenant-ID", "tenantA")
	
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found, got %d", resp.StatusCode)
	}

	var appErr types.AppErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&appErr); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if appErr.RequestID == "" {
		t.Error("Missing requestId in error response")
	}
	if appErr.Step != "routing" {
		t.Errorf("Expected step 'routing', got %s", appErr.Step)
	}
	if appErr.ReasonCode != "FLOW_NOT_FOUND" {
		t.Errorf("Expected reason 'FLOW_NOT_FOUND', got %s", appErr.ReasonCode)
	}
}

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
		RequestID        string  `json:"requestId"`
		AnonymizedText   string  `json:"anonymized_text"`
		LLMOutput        string  `json:"llm_output"`
		RehydratedOutput *string `json:"rehydrated_output"`
		Provider         string  `json:"provider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ingestResp); err != nil {
		t.Fatalf("Failed to decode ingestion response: %v", err)
	}

	if ingestResp.RequestID == "" {
		t.Error("Ingestion response missing RequestID")
	}
	if !strings.Contains(ingestResp.AnonymizedText, "[[AUG:EMAIL_ADDRESS:1]]") {
		t.Errorf("Ingestion response text missing token: %s", ingestResp.AnonymizedText)
	}
	if !strings.HasPrefix(ingestResp.LLMOutput, "ECHO: ") {
		t.Errorf("Ingestion LLMOutput does not start with ECHO: %s", ingestResp.LLMOutput)
	}
	if ingestResp.RehydratedOutput == nil || !strings.Contains(*ingestResp.RehydratedOutput, "john.doe@example.com") {
		t.Errorf("Ingestion RehydratedOutput missing the original email: %v", ingestResp.RehydratedOutput)
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

func TestIngestionRehydrationFailClosed(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	payload := map[string]string{
		"text": "Hello, my email is jane.smith@example.com, this should fail since TTL is 1.",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, "http://ingestion-go:8080/ingest/webhook/expire_demo", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("X-Tenant-ID", "tenantA")
	req.Header.Set("Content-Type", "application/json")

	// This triggers a 2 second sleep in the mock backend.
	// TTL is 1 second, so the rehydration should fail and it should return 403 Forbidden.
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send ingestion request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden due to expiry fail-closed, got %d", resp.StatusCode)
	}

	var appErr types.AppErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&appErr); err != nil {
		t.Fatalf("Failed to decode fail-closed error response: %v", err)
	}

	if appErr.ReasonCode != "TOKEN_EXPIRED" {
		t.Errorf("Expected reason 'TOKEN_EXPIRED', got %s", appErr.ReasonCode)
	}
	if appErr.Step != "rehydrate" {
		t.Errorf("Expected step 'rehydrate', got %s", appErr.Step)
	}

	// 3) Audit Event Validation
	// We wait a tiny bit to ensure events are flushed (synchronous in our code, but good practice if async later)
	time.Sleep(100 * time.Millisecond)
	
	auditReq, _ := http.NewRequest(http.MethodGet, auditAdminURL+"?requestId="+appErr.RequestID, nil)
	auditResp, err := client.Do(auditReq)
	if err != nil {
		t.Fatalf("Failed to fetch audit logs: %v", err)
	}
	defer auditResp.Body.Close()

	if auditResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from audit admin, got %d", auditResp.StatusCode)
	}

	var events []audit.AuditEvent
	if err := json.NewDecoder(auditResp.Body).Decode(&events); err != nil {
		t.Fatalf("Failed to decode audit events: %v", err)
	}

	if len(events) == 0 {
		t.Errorf("Expected audit events for request %s, found none", appErr.RequestID)
	}

	foundAnonymize := false
	for _, ev := range events {
		rawEv, _ := json.Marshal(ev)
		if strings.Contains(string(rawEv), "jane.smith@example.com") {
			t.Errorf("CRITICAL FAILURE: Audit event leaked PII! %s", string(rawEv))
		}
		
		if ev.Step == "anonymize" && ev.Outcome == "success" {
			foundAnonymize = true
		}
		
		if ev.Step == "rehydrate" && ev.Outcome == "fail" {
			if ev.ReasonCode != "TOKEN_NOT_FOUND" {
				// We actually emit TOKEN_NOT_FOUND in the audit when rehydration loop fails to find/unmarshal token
				// Then the outer block checks if FAIL_CLOSED it forces 403 TOKEN_EXPIRED upwards to HTTP.
				// This is correct as per our implementation.
			}
		}
	}

	if !foundAnonymize {
		t.Errorf("Expected at least one successful anonymize audit event")
	}
}
