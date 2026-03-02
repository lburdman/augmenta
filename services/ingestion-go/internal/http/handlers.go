package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/privacy"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

type Server struct {
	mux       *http.ServeMux
	flows     map[string]types.FlowConfig
	apiClient *privacy.Client
}

// key defines the lookup format for a tenant+source config.
func flowKey(tenantID, sourceID string) string {
	return tenantID + ":" + sourceID
}

func NewServer(flowCfgs []types.FlowConfig, apiClient *privacy.Client) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		flows:     make(map[string]types.FlowConfig),
		apiClient: apiClient,
	}

	// Index flows for fast lookup
	for _, f := range flowCfgs {
		s.flows[flowKey(f.TenantID, f.SourceID)] = f
	}

	s.mux.HandleFunc("GET /health", s.handleHealth())
	s.mux.HandleFunc("POST /ingest/webhook/", s.handleIngest())

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (s *Server) handleIngest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		w.Header().Set("Content-Type", "application/json")

		// 1. Basic validation
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			writeError(w, http.StatusBadRequest, "Missing X-Tenant-ID header")
			return
		}

		// Extract sourceId from the path (/ingest/webhook/{sourceId})
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ingest/webhook/"), "/")
		if len(pathParts) == 0 || pathParts[0] == "" {
			writeError(w, http.StatusBadRequest, "Missing sourceId in path")
			return
		}
		sourceID := pathParts[0]

		var req types.IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		reqID := uuid.New().String()

		// 2. Load flow routing
		flowCfg, exists := s.flows[flowKey(tenantID, sourceID)]
		if !exists {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=routing status=not_found", reqID, tenantID, sourceID)
			writeError(w, http.StatusNotFound, "Flow configuration not found for tenant and source")
			return
		}

		// 3. Privacy Anonymize
		privacyReq := types.PrivacyAnonymizeRequest{
			RequestID: reqID,
			TenantID:  tenantID,
			Text:      req.Text,
			Operators: flowCfg.Operators,
		}

		// DO NOT log the raw text content!
		log.Printf("reqId=%s tenantId=%s sourceId=%s step=anonymize status=started", reqID, tenantID, sourceID)
		
		privacyResp, err := s.apiClient.Anonymize(r.Context(), privacyReq)
		if err != nil {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=anonymize status=error err=%q", reqID, tenantID, sourceID, err)
			writeError(w, http.StatusInternalServerError, "Failed to anonymize payload")
			return
		}

		// 4. LLM Gateway Forwarding
		// ONLY send the anonymized text
		llmReq := types.LLMGatewayRequest{
			RequestID: reqID,
			TenantID:  tenantID,
			SourceID:  sourceID,
			Prompt:    privacyResp.AnonymizedText,
		}

		log.Printf("reqId=%s tenantId=%s sourceId=%s step=llm_gateway status=started", reqID, tenantID, sourceID)
		llmResp, err := s.apiClient.CompleteLLM(r.Context(), llmReq)
		if err != nil {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=llm_gateway status=error err=%q", reqID, tenantID, sourceID, err)
			writeError(w, http.StatusBadGateway, "Failed to complete request via LLM gateway")
			return
		}

		latency := time.Since(startTime)
		log.Printf("reqId=%s tenantId=%s sourceId=%s step=completed status=success latency_ms=%d", 
			reqID, tenantID, sourceID, latency.Milliseconds())

		// 5. Response
		resp := types.IngestResponse{
			RequestID:      reqID,
			TenantID:       tenantID,
			SourceID:       sourceID,
			AnonymizedText: privacyResp.AnonymizedText,
			LLMOutput:      llmResp.Output,
			Provider:       "echo",
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("reqId=%s step=response status=error err=%q", reqID, err)
		}
	}
}
