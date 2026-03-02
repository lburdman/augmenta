package types

// OperatorParams matches the operator config for the privacy service.
type OperatorParams struct {
	Type     string `json:"type" yaml:"type"`
	NewValue string `json:"new_value" yaml:"new_value"`
}

// FlowConfig defines the configuration for a single source webhook flow.
type FlowConfig struct {
	TenantID          string                    `yaml:"tenantId"`
	SourceID          string                    `yaml:"sourceId"`
	TTLSeconds        int                       `yaml:"ttlSeconds"`
	RehydrationPolicy string                    `yaml:"rehydrationPolicy"`
	FailClosed        bool                      `yaml:"failClosed"`
	Operators         map[string]OperatorParams `yaml:"operators"`
}

// ConfigFile represents the root of the flows.yaml configuration.
type ConfigFile struct {
	Flows []FlowConfig `yaml:"flows"`
}

// IngestRequest represents the incoming webhook payload.
type IngestRequest struct {
	Text string `json:"text"`
}

// IngestResponse represents the response back to the caller.
type IngestResponse struct {
	RequestID        string  `json:"requestId"`
	TenantID         string  `json:"tenantId"`
	SourceID         string  `json:"sourceId"`
	AnonymizedText   string  `json:"anonymized_text,omitempty"`
	LLMOutput        string  `json:"llm_output,omitempty"`
	RehydratedOutput *string `json:"rehydrated_output"`
	Rehydration      string  `json:"rehydration,omitempty"`
	TTLSeconds       int     `json:"ttlSeconds,omitempty"`
	Provider         string  `json:"provider,omitempty"`
}

// AppErrorResponse is the standardized controlled error structure returned to clients.
type AppErrorResponse struct {
	RequestID  string `json:"requestId"`
	TenantID   string `json:"tenantId,omitempty"`
	SourceID   string `json:"sourceId,omitempty"`
	Step       string `json:"step,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
	Message    string `json:"message"`
}

// PrivacyAnonymizeRequest matches the payload expected by privacy-service/anonymize.
type PrivacyAnonymizeRequest struct {
	RequestID string                    `json:"requestId"`
	TenantID  string                    `json:"tenantId"`
	Text      string                    `json:"text"`
	Operators map[string]OperatorParams `json:"operators"`
}

// EntityMapping represents a token mapped to its original plaintext value
type EntityMapping struct {
	Token      string `json:"token"`
	EntityType string `json:"entity_type"`
	Original   string `json:"original"`
}

// PrivacyAnonymizeResponse represents what is returned by the privacy service.
type PrivacyAnonymizeResponse struct {
	AnonymizedText  string          `json:"anonymized_text"`
	Mappings        []EntityMapping `json:"mappings"`
	AnalyzerResults []interface{}   `json:"analyzer_results"`
	Stats           interface{}     `json:"stats"`
}

// LLMGatewayRequest is what we forward to llm-gateway-go
type LLMGatewayRequest struct {
	RequestID string `json:"requestId"`
	TenantID  string `json:"tenantId"`
	SourceID  string `json:"sourceId"`
	Prompt    string `json:"prompt"`
}

// LLMGatewayResponse is what we receive from llm-gateway-go
type LLMGatewayResponse struct {
	RequestID string `json:"requestId"`
	Output    string `json:"output"`
}
