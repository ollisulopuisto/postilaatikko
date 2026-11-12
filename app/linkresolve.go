package app

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"time"

	"github.com/hkdb/aerion/internal/linkresolve"
)

// LinkChainResult is the redirect chain returned to the frontend for the
// "Open via Cloudflare DNS" link action. Error carries a partial-chain
// failure while keeping the hops gathered before it; Wails discards the
// result value when a Go error is returned, so chain failures travel here.
type LinkChainResult struct {
	Hops     []linkresolve.Hop `json:"hops"`
	FinalURL string            `json:"finalURL"`
	Error    string            `json:"error,omitempty"`
}

// ResolveLinkViaCloudflareDNS resolves the link's host through Cloudflare's
// DNS-over-HTTPS resolver (1.1.1.1), bypassing local DNS-based ad blocking,
// and follows the redirect chain without cookies so the user can see (and
// open) the real destination. GET-only, hop-capped, and guarded against
// non-public dial targets. Only runs on explicit user request.
func (a *App) ResolveLinkViaCloudflareDNS(rawURL string) (*LinkChainResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http(s) links can be resolved")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	resolver := &linkresolve.Resolver{
		Lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return linkresolve.ResolveDoH(ctx, linkresolve.DefaultDoHURL, host)
		},
		MaxHops: linkresolve.DefaultMaxHops,
	}
	result, err := resolver.Follow(ctx, rawURL)
	chain := &LinkChainResult{}
	if result != nil {
		chain.Hops = result.Hops
		chain.FinalURL = result.FinalURL
	}
	if err != nil {
		chain.Error = err.Error()
	}
	return chain, nil
}
