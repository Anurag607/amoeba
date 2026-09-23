// Package execution defines portable run identity, tool-attempt, approval,
// and versioned dispatch contracts. Persistence is supplied by the host.
package execution

import (
	"context"
	"fmt"
	"strings"
)

// Identity is the immutable server-admitted execution identity inherited by
// child work. It must never be populated from model tool arguments.
type Identity struct {
	PrincipalID string `json:"principal_id"`
	TenantID    string `json:"tenant_id,omitempty"`
	SessionID   string `json:"session_id"`
	RunID       string `json:"run_id"`
}

func (i Identity) Validate() error {
	if strings.TrimSpace(i.PrincipalID) == "" || strings.TrimSpace(i.SessionID) == "" || strings.TrimSpace(i.RunID) == "" {
		return fmt.Errorf("execution identity requires principal, session, and run IDs")
	}
	return nil
}

type identityKey struct{}

// WithIdentity pins identity once. Rebinding an existing context fails.
func WithIdentity(ctx context.Context, identity Identity) (context.Context, error) {
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	if existing, ok := IdentityFromContext(ctx); ok && existing != identity {
		return nil, fmt.Errorf("execution identity is already pinned")
	}
	return context.WithValue(ctx, identityKey{}, identity), nil
}

// IdentityFromContext returns the admitted identity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}
