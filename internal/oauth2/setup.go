package oauth2

import (
	"context"
	"errors"
)

// WaitForSetupCallback performs the existing PKCE callback flow but requires a
// provider identity. Provider error bodies are not returned to the terminal.
func (m *Manager) WaitForSetupCallback(ctx context.Context) (*TokenResponse, string, error) {
	tokens, email, err := m.WaitForCallback(ctx)
	if err != nil {
		return nil, "", errors.New("OAuth sign-in did not complete; retry consent")
	}
	if tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.ExpiresIn <= 0 || email == "" {
		tokens.Clear()
		return nil, "", errors.New("OAuth sign-in requires an account identity and offline refresh grant")
	}
	return tokens, email, nil
}
