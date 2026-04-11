package adapters

import (
	"context"

	"github.com/penche/router/internal/domain"
)

// DeliveryResult holds metadata returned after a successful send.
type DeliveryResult struct {
	// ExternalID is the ID assigned by the target system (e.g., Taiga issue ref).
	ExternalID string
	// Message is a human-readable summary.
	Message string
}

// DestinationAdapter is the contract every target integration must satisfy.
type DestinationAdapter interface {
	// Name returns the unique adapter identifier (matches config key).
	Name() string
	// Send delivers the event to the target system.
	Send(ctx context.Context, evt *domain.StoredEvent) (DeliveryResult, error)
	// ValidateConfig checks that the adapter is properly configured.
	ValidateConfig() error
}
