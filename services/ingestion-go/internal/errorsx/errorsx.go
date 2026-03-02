package errorsx

import (
	"fmt"
	"net/http"
)

// Step indicates which part of the pipeline failed.
type Step string

const (
	StepIngest   Step = "ingest"
	StepRouting  Step = "routing"
	StepAnonymize Step = "anonymize"
	StepVaultPut Step = "vault_put"
	StepLLMCall  Step = "llm_call"
	StepRehydrate Step = "rehydrate"
)

// ReasonCode is a standardized, enumerable reason for failure.
type ReasonCode string

const (
	ReasonInputInvalid   ReasonCode = "INPUT_INVALID"
	ReasonFlowNotFound   ReasonCode = "FLOW_NOT_FOUND"
	ReasonPrivacyUnavail ReasonCode = "PRIVACY_UNAVAILABLE"
	ReasonPrivacyError   ReasonCode = "PRIVACY_ERROR"
	ReasonVaultUnavail   ReasonCode = "VAULT_UNAVAILABLE"
	ReasonVaultWriteFail ReasonCode = "VAULT_WRITE_FAILED"
	ReasonVaultReadFail  ReasonCode = "VAULT_READ_FAILED"
	ReasonTokenExpired   ReasonCode = "TOKEN_EXPIRED"
	ReasonTokenNotFound  ReasonCode = "TOKEN_NOT_FOUND"
	ReasonTenantMismatch ReasonCode = "TENANT_MISMATCH"
	ReasonLLMUnavail     ReasonCode = "LLM_UNAVAILABLE"
	ReasonLLMError       ReasonCode = "LLM_ERROR"
	ReasonPolicyBlocked  ReasonCode = "POLICY_BLOCKED"
	ReasonInternalError  ReasonCode = "INTERNAL_ERROR"
)

// AppError carries all necessary metadata to surface a controlled JSON response
// without leaking PII or internal stack traces to the API caller.
type AppError struct {
	Status      int
	Step        Step
	Reason      ReasonCode
	SafeMessage string
	Cause       error
}

// Error implements the error interface natively.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %s (cause: %v)", e.Step, e.Reason, e.SafeMessage, e.Cause)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Step, e.Reason, e.SafeMessage)
}

// New constructs a new formatted AppError payload.
func New(step Step, reason ReasonCode, status int, safeMsg string, cause error) *AppError {
	return &AppError{
		Status:      status,
		Step:        step,
		Reason:      reason,
		SafeMessage: safeMsg,
		Cause:       cause,
	}
}

// Helper Constructors

func NewInputInvalid(msg string, cause error) *AppError {
	return New(StepIngest, ReasonInputInvalid, http.StatusBadRequest, msg, cause)
}

func NewInternalError(step Step, cause error) *AppError {
	return New(step, ReasonInternalError, http.StatusInternalServerError, "An unexpected internal error occurred", cause)
}
