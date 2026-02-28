package secrets

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a secret is not found in an external backend.
var ErrNotFound = errors.New("secret not found")

// ExternalBackend defines an interface for external secret storage systems
// (e.g., HashiCorp Vault, AWS Secrets Manager). Implementations can be
// registered and passed to a SecretStore via store options.
type ExternalBackend interface {
	GetSecret(ctx context.Context, name string) (string, error)
	SetSecret(ctx context.Context, name string, value string) error
	DeleteSecret(ctx context.Context, name string) error
	Ping(ctx context.Context) error
	Name() string
}
