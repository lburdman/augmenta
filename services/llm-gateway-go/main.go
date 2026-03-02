package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type CompleteRequest struct {
	RequestID string `json:"requestId"`
	TenantID  string `json:"tenantId"`
	SourceID  string `json:"sourceId"`
	Prompt    string `json:"prompt"`
}

type CompleteResponse struct {
	RequestID string `json:"requestId"`
	Output    string `json:"output"`
}

type LastResponse struct {
	RequestID  string `json:"requestId"`
	TenantID   string `json:"tenantId"`
	SourceID   string `json:"sourceId"`
	PromptHash string `json:"prompt_hash"`
}

var (
	lastRequestMutex sync.RWMutex
	lastRequestMeta  *LastResponse
)

func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleComplete(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Content-Type", "application/json")

	var req CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.RequestID == "" || req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "Missing requestId or prompt")
		return
	}

	// Calculate prompt hash but NEVER log the raw prompt
	hash := sha256.Sum256([]byte(req.Prompt))
	hashStr := hex.EncodeToString(hash[:])

	log.Printf("reqId=%s tenantId=%s sourceId=%s step=complete status=started", req.RequestID, req.TenantID, req.SourceID)

	// Save debug metadata in-memory safely
	lastRequestMutex.Lock()
	lastRequestMeta = &LastResponse{
		RequestID:  req.RequestID,
		TenantID:   req.TenantID,
		SourceID:   req.SourceID,
		PromptHash: hashStr,
	}
	lastRequestMutex.Unlock()

	if req.SourceID == "expire_demo" {
		time.Sleep(2 * time.Second)
	}

	// Pretend to call an LLM with "ECHO:" provider stub
	output := "ECHO: " + req.Prompt

	resp := CompleteResponse{
		RequestID: req.RequestID,
		Output:    output,
	}

	latency := time.Since(startTime)
	log.Printf("reqId=%s tenantId=%s sourceId=%s step=completed status=success latency_ms=%d",
		req.RequestID, req.TenantID, req.SourceID, latency.Milliseconds())

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("reqId=%s step=response status=error err=%q", req.RequestID, err)
	}
}

func handleLast(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	lastRequestMutex.RLock()
	defer lastRequestMutex.RUnlock()

	if lastRequestMeta == nil {
		writeError(w, http.StatusNotFound, "No messages received yet")
		return
	}

	_ = json.NewEncoder(w).Encode(lastRequestMeta)
}

func main() {
	log.SetFlags(0)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /complete", handleComplete)
	mux.HandleFunc("GET /last", handleLast)

	port := os.Getenv("PORT")
	if port == "" {
		port = "7000"
	}

	log.Printf("Starting LLM Gateway Echo Provider on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
