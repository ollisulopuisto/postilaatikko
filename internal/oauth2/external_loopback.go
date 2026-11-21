package oauth2

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ExternalLoopbackFlow retains PKCE material on the account-owning host while
// another device forwards only its loopback callback code and state. It is not
// serializable and exposes no verifier, provider secret or tokens to that device.
type ExternalLoopbackFlow struct {
	mu                        sync.Mutex
	provider                  ProviderConfig
	verifier, state, redirect string
	expires                   time.Time
	consumed                  bool
}

// NewExternalLoopbackFlow builds a five-minute flow for a desktop IPv4 callback.
// Only an explicit ephemeral loopback port and /callback path are accepted.
func NewExternalLoopbackFlow(provider ProviderConfig, redirect string) (*ExternalLoopbackFlow, string, error) {
	u, err := url.Parse(redirect)
	if err != nil {
		return nil, "", fmt.Errorf("invalid desktop callback")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "/callback" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, "", fmt.Errorf("desktop callback must use IPv4 loopback /callback with an explicit port")
	}
	if provider.ClientID == "" || provider.Name != "google" {
		return nil, "", fmt.Errorf("configured Google Mail project required")
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, "", err
	}
	state, err := generateState()
	if err != nil {
		return nil, "", err
	}
	flow := &ExternalLoopbackFlow{provider: provider, verifier: verifier, state: state, redirect: redirect, expires: time.Now().Add(5 * time.Minute)}
	return flow, buildAuthURL(provider, state, generateCodeChallenge(verifier), redirect), nil
}

// Exchange consumes the code once with host-retained PKCE. Identity must still
// be verified by the account owner against the returned access token.
func (f *ExternalLoopbackFlow) Exchange(ctx context.Context, code, state string) (*TokenResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	if f.consumed || time.Now().After(f.expires) {
		f.mu.Unlock()
		return nil, fmt.Errorf("consent expired or already consumed")
	}
	if code == "" || len(code) > 8192 || len(state) != len(f.state) || subtle.ConstantTimeCompare([]byte(state), []byte(f.state)) != 1 {
		f.mu.Unlock()
		return nil, fmt.Errorf("consent callback state mismatch")
	}
	f.consumed = true
	provider, verifier, redirect := f.provider, f.verifier, f.redirect
	f.verifier = ""
	f.state = ""
	f.provider = ProviderConfig{}
	f.mu.Unlock()
	tokens, err := NewManager().exchangeCodeContext(ctx, provider, code, verifier, redirect)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		tokens.Clear()
		return nil, err
	}
	return tokens, nil
}

// Cancel discards all retained flow material. A cancelled flow cannot exchange.
func (f *ExternalLoopbackFlow) Cancel() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumed = true
	f.verifier = ""
	f.state = ""
	f.provider = ProviderConfig{}
}
