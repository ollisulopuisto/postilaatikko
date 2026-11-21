package oauth2

import (
	"context"
	"fmt"
)

// VerifyGoogleFilterRefresh proves settings access survives the next refresh.
// Missing reported scopes are unverified, not evidence of durable access.
func (m *Manager) VerifyGoogleFilterRefresh(ctx context.Context, provider ProviderConfig, refreshToken string, required []string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("durable Google refresh grant missing")
	}
	tokens, err := m.RefreshTokenWithProviderContext(ctx, provider, refreshToken)
	if err != nil {
		return nil, err
	}
	// Identity was checked on the authorization token; this request uses its
	// refresh grant and the same pinned client rather than caller-supplied email.
	if tokens.ExpiresIn <= 0 {
		tokens.Clear()
		return nil, fmt.Errorf("refreshed Google grant already expired")
	}
	if err := ValidateGoogleFilterGrant("verified", "verified", required, tokens); err != nil {
		tokens.Clear()
		return nil, err
	}
	return tokens, nil
}
