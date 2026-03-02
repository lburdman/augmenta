package vault

import (
	"context"

	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

// Vault interface defines the storage layout for anonymization token mappings.
type Vault interface {
	// PutMappings stores an array of entity mappings.
	PutMappings(ctx context.Context, tenantID, requestID string, ttlSeconds int, mappings []types.EntityMapping) error

	// GetOriginal retrieves the original value for a given token. Returns empty string if missing or expired.
	GetOriginal(ctx context.Context, tenantID, requestID, token string) (string, error)
}
