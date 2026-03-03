package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/audit"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/errorsx"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/privacy"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/vault"
)

// Server dependencies
type Server struct {
	mux       *http.ServeMux
	flows     map[string]types.FlowConfig
	apiClient   *privacy.Client
	vault       vault.Vault
	auditLogger audit.Logger
}

// key defines the lookup format for a tenant+source config.
func flowKey(tenantID, sourceID string) string {
	return tenantID + ":" + sourceID
}

func NewServer(flowCfgs []types.FlowConfig, apiClient *privacy.Client, vlt vault.Vault, auditLogger audit.Logger) *Server {
	s := &Server{
		mux:         http.NewServeMux(),
		flows:       make(map[string]types.FlowConfig),
		apiClient:   apiClient,
		vault:       vlt,
		auditLogger: auditLogger,
	}

	// Index flows for fast lookup
	for _, f := range flowCfgs {
		s.flows[flowKey(f.TenantID, f.SourceID)] = f
	}

	s.mux.HandleFunc("GET /health", s.handleHealth())
	s.mux.HandleFunc("POST /ingest/webhook/", s.handleIngest())

	if s.auditLogger != nil {
		s.mux.HandleFunc("GET /admin/audit", s.handleAudit())
	}

	return s
}

// corsMiddleware wraps the assigned handler to inject permissive Cross-Origin headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Tenant-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(s.mux).ServeHTTP(w, r)
}

func writeAppError(w http.ResponseWriter, reqID, tenantID, sourceID string, appErr *errorsx.AppError) {
	w.WriteHeader(appErr.Status)
	resp := types.AppErrorResponse{
		RequestID:  reqID,
		TenantID:   tenantID,
		SourceID:   sourceID,
		Step:       string(appErr.Step),
		ReasonCode: string(appErr.Reason),
		Message:    appErr.SafeMessage,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (s *Server) handleAudit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := r.URL.Query().Get("requestId")
		
		events := s.auditLogger.GetRecent()
		
		var filtered []audit.AuditEvent
		if reqID != "" {
			for _, ev := range events {
				if ev.RequestID == reqID {
					filtered = append(filtered, ev)
				}
			}
		} else {
			filtered = events
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(filtered)
	}
}

func (s *Server) handleIngest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		w.Header().Set("Content-Type", "application/json")

		reqID := uuid.New().String()

		// 1. Basic validation
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			writeAppError(w, reqID, "", "", errorsx.NewInputInvalid("Missing X-Tenant-ID header", nil))
			return
		}

		// Extract sourceId from the path (/ingest/webhook/{sourceId})
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ingest/webhook/"), "/")
		if len(pathParts) == 0 || pathParts[0] == "" {
			writeAppError(w, reqID, tenantID, "", errorsx.NewInputInvalid("Missing sourceId in path", nil))
			return
		}
		sourceID := pathParts[0]

		var req types.IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, reqID, tenantID, sourceID, errorsx.NewInputInvalid("Invalid JSON payload", err))
			return
		}

		// 2. Load flow routing
		flowCfg, exists := s.flows[flowKey(tenantID, sourceID)]
		if !exists {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=routing status=not_found", reqID, tenantID, sourceID)
			writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepRouting, errorsx.ReasonFlowNotFound, http.StatusNotFound, "Flow configuration not found for tenant and source", nil))
			return
		}

		// 3. Privacy Anonymize
		privacyReq := types.PrivacyAnonymizeRequest{
			RequestID: reqID,
			TenantID:  tenantID,
			Text:      req.Text,
			Operators: flowCfg.Operators,
		}

		log.Printf("reqId=%s tenantId=%s sourceId=%s step=anonymize status=started", reqID, tenantID, sourceID)
		
		anonymizeStart := time.Now()
		privacyResp, err := s.apiClient.Anonymize(r.Context(), privacyReq)
		anonymizeLat := time.Since(anonymizeStart).Milliseconds()
		
		if err != nil {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=anonymize status=error err=%q", reqID, tenantID, sourceID, err)
			
			if s.auditLogger != nil {
				s.auditLogger.Emit(audit.AuditEvent{
					RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepAnonymize),
					Outcome: "fail", ReasonCode: string(errorsx.ReasonPrivacyUnavail), LatencyMs: anonymizeLat,
				})
			}

			if flowCfg.FailClosed {
				writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepAnonymize, errorsx.ReasonPrivacyUnavail, http.StatusBadGateway, "Privacy service unavailable or returned an error", err))
				return
			}
			writeAppError(w, reqID, tenantID, sourceID, errorsx.NewInternalError(errorsx.StepAnonymize, err))
			return
		}

		if s.auditLogger != nil {
			metrics := make(map[string]any)
			if privacyResp.Stats != nil {
				metrics["stats"] = privacyResp.Stats
			}
			metrics["tokens_total"] = len(privacyResp.Mappings)
			s.auditLogger.Emit(audit.AuditEvent{
				RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepAnonymize),
				Outcome: "success", LatencyMs: anonymizeLat, Metrics: metrics,
			})
		}

		// 3.5 Store Mappings in Vault
		if s.vault != nil && len(privacyResp.Mappings) > 0 {
			ttl := flowCfg.TTLSeconds
			if ttl <= 0 {
				ttl = 3600 // default
			}
			vaultStart := time.Now()
			err := s.vault.PutMappings(r.Context(), tenantID, reqID, ttl, privacyResp.Mappings)
			vaultLat := time.Since(vaultStart).Milliseconds()

			if err != nil {
				log.Printf("reqId=%s step=vault status=error err=%q", reqID, err)
				if s.auditLogger != nil {
					s.auditLogger.Emit(audit.AuditEvent{
						RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepVaultPut),
						Outcome: "fail", ReasonCode: string(errorsx.ReasonVaultWriteFail), LatencyMs: vaultLat,
					})
				}
				if flowCfg.FailClosed {
					writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepVaultPut, errorsx.ReasonVaultWriteFail, http.StatusServiceUnavailable, "Failed to secure anonymized mappings", err))
					return
				}
				writeAppError(w, reqID, tenantID, sourceID, errorsx.NewInternalError(errorsx.StepVaultPut, err))
				return
			}
			if s.auditLogger != nil {
				s.auditLogger.Emit(audit.AuditEvent{
					RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepVaultPut),
					Outcome: "success", LatencyMs: vaultLat, Metrics: map[string]any{"tokens_written": len(privacyResp.Mappings), "ttl_seconds": ttl},
				})
			}
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
		llmStart := time.Now()
		llmResp, err := s.apiClient.CompleteLLM(r.Context(), llmReq)
		llmLat := time.Since(llmStart).Milliseconds()
		
		if err != nil {
			log.Printf("reqId=%s tenantId=%s sourceId=%s step=llm_gateway status=error err=%q", reqID, tenantID, sourceID, err)
			
			if s.auditLogger != nil {
				s.auditLogger.Emit(audit.AuditEvent{
					RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepLLMCall),
					Outcome: "fail", ReasonCode: string(errorsx.ReasonLLMUnavail), LatencyMs: llmLat,
				})
			}
			if flowCfg.FailClosed {
				writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepLLMCall, errorsx.ReasonLLMUnavail, http.StatusBadGateway, "Failed to complete request via LLM gateway", err))
				return
			}
			writeAppError(w, reqID, tenantID, sourceID, errorsx.NewInternalError(errorsx.StepLLMCall, err))
			return
		}
		
		if s.auditLogger != nil {
			// Using mock prompt hash and provider "echo" since client hash logic is in Gateway. Could locally hash too.
			s.auditLogger.Emit(audit.AuditEvent{
				RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepLLMCall),
				Outcome: "success", LatencyMs: llmLat, Metrics: map[string]any{"provider": "echo", "output_len": len(llmResp.Output)},
			})
		}

		// 5. Rehydration
		var rehydratedOutput *string
		rehydrationStatus := "skipped"
		if flowCfg.RehydrationPolicy == "on_success" && s.vault != nil {
			var vaultLat int64
			rehydrateStart := time.Now()
			rehydrated := llmResp.Output
			failedRehydration := false

			tokensFound := 0
			// Rehydrate all original mappings dynamically
			for _, m := range privacyResp.Mappings {
				if strings.Contains(rehydrated, m.Token) {
					orig, err := s.vault.GetOriginal(r.Context(), tenantID, reqID, m.Token)
					if err != nil || orig == "" {
						log.Printf("reqId=%s step=rehydration status=token_missing_or_expired token=%s err=%v", reqID, m.Token, err)
						failedRehydration = true
						break
					}
					rehydrated = strings.ReplaceAll(rehydrated, m.Token, orig)
					tokensFound++
				}
			}
			vaultLat = time.Since(rehydrateStart).Milliseconds()

			if failedRehydration {
				if s.auditLogger != nil {
					s.auditLogger.Emit(audit.AuditEvent{
						RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepRehydrate),
						Outcome: "fail", ReasonCode: string(errorsx.ReasonTokenNotFound), LatencyMs: vaultLat,
						Metrics: map[string]any{"tokens_found": tokensFound, "tokens_missing": len(privacyResp.Mappings) - tokensFound},
					})
				}
				if flowCfg.FailClosed {
					writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepRehydrate, errorsx.ReasonTokenExpired, http.StatusForbidden, "Rehydration failed due to expired or missing token", nil))
					return
				}
				writeAppError(w, reqID, tenantID, sourceID, errorsx.New(errorsx.StepRehydrate, errorsx.ReasonTokenNotFound, http.StatusInternalServerError, "Failed to rehydrate mappings", nil))
				return
			} else {
				rehydratedOutput = &rehydrated
				rehydrationStatus = "performed"
				if s.auditLogger != nil {
					s.auditLogger.Emit(audit.AuditEvent{
						RequestID: reqID, TenantID: tenantID, SourceID: sourceID, Step: string(errorsx.StepRehydrate),
						Outcome: "success", LatencyMs: vaultLat,
						Metrics: map[string]any{"tokens_found": tokensFound, "performed": true},
					})
				}
			}
		}

		latency := time.Since(startTime)
		log.Printf("reqId=%s tenantId=%s sourceId=%s step=completed status=success latency_ms=%d", 
			reqID, tenantID, sourceID, latency.Milliseconds())

		// 6. Response
		resp := types.IngestResponse{
			RequestID:        reqID,
			TenantID:         tenantID,
			SourceID:         sourceID,
			AnonymizedText:   privacyResp.AnonymizedText,
			LLMOutput:        llmResp.Output,
			RehydratedOutput: rehydratedOutput,
			Rehydration:      rehydrationStatus,
			TTLSeconds:       flowCfg.TTLSeconds,
			Provider:         "echo",
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("reqId=%s step=response status=error err=%q", reqID, err)
		}
	}
}
