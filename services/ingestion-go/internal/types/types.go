package types

// OperatorParams matches the operator config for the privacy service.
type OperatorParams struct {
	Type     string `json:"type" yaml:"type"`
	NewValue string `json:"new_value" yaml:"new_value"`
}

// FlowConfig defines the configuration for a single source webhook flow.
type FlowConfig struct {
	TenantID  string                    `yaml:"tenantId"`
	SourceID  string                    `yaml:"sourceId"`
	Operators map[string]OperatorParams `yaml:"operators"`
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
	RequestID        string `json:"requestId"`
	TenantID         string `json:"tenantId"`
	SourceID         string `json:"sourceId"`
	AnonymizedText   string `json:"anonymized_text,omitempty"`
	DownstreamStatus int    `json:"downstream_status,omitempty"`
}

// PrivacyAnonymizeRequest matches the payload expected by privacy-service/anonymize.
type PrivacyAnonymizeRequest struct {
	RequestID string                    `json:"requestId"`
	TenantID  string                    `json:"tenantId"`
	Text      string                    `json:"text"`
	Operators map[string]OperatorParams `json:"operators"`
}

// PrivacyAnonymizeResponse represents what is returned by the privacy service.
type PrivacyAnonymizeResponse struct {
	AnonymizedText  string        `json:"anonymized_text"`
	AnalyzerResults []interface{} `json:"analyzer_results"`
	Stats           interface{}   `json:"stats"`
}

// DownstreamReceiveRequest is what we forward to downstream-mock.
type DownstreamReceiveRequest struct {
	RequestID      string `json:"requestId"`
	TenantID       string `json:"tenantId"`
	SourceID       string `json:"sourceId"`
	AnonymizedText string `json:"anonymized_text"`
}
