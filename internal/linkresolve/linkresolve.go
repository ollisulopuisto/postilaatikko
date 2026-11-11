// Package linkresolve resolves link hosts via DNS-over-HTTPS and follows
// redirect chains without cookies, so a user can inspect where a tracking
// link really goes and open the final destination even when a local ad
// blocker blocks the tracking domain's DNS.
//
// The fetcher is a server-side request surface and is hardened accordingly:
// only http(s) on ports 80/443, DNS answers pointing at loopback, private,
// link-local or otherwise non-public addresses are refused, redirect hops are
// capped, and response bodies are discarded after a small read.
package linkresolve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// DefaultDoHURL is Cloudflare's public DNS-over-HTTPS JSON endpoint.
const DefaultDoHURL = "https://1.1.1.1/dns-query"

// DefaultMaxHops caps the redirect chain length.
const DefaultMaxHops = 10

// Hop is one URL in the chain and the status code the server returned for
// it. The final entry carries the destination's response status; intermediate
// entries carry the redirect status (301/302/303/307/308).
type Hop struct {
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
}

// Result is the outcome of following a link: every hop visited and the final
// destination URL. On error the hops gathered so far are preserved.
type Result struct {
	Hops     []Hop  `json:"hops"`
	FinalURL string `json:"finalURL"`
}

// LookupFunc resolves a host name to IP addresses.
type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// Resolver follows link redirect chains, resolving hosts through a caller
// supplied lookup (DNS-over-HTTPS in production).
type Resolver struct {
	// Lookup resolves host names; production uses Cloudflare DoH.
	Lookup LookupFunc
	// AllowPrivate permits loopback/private/link-local answers. Tests only.
	AllowPrivate bool
	// AllowAnyPort permits ports other than 80/443. Tests only.
	AllowAnyPort bool
	// MaxHops caps the chain; zero uses DefaultMaxHops.
	MaxHops int
	// PerHopTimeout bounds each request; zero uses 8 seconds.
	PerHopTimeout time.Duration
}

// doHAnswer is the subset of the DNS JSON API response we read.
type doHResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// ResolveDoH resolves host through the DNS-over-HTTPS JSON API, preferring A
// records and falling back to AAAA.
func ResolveDoH(ctx context.Context, dohURL, host string) ([]netip.Addr, error) {
	for _, qtype := range []string{"A", "AAAA"} {
		u := fmt.Sprintf("%s?name=%s&type=%s", dohURL, url.QueryEscape(host), qtype)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("DoH query for %s: %w", host, err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading DoH answer for %s: %w", host, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("DoH query for %s: HTTP %d", host, resp.StatusCode)
		}
		var parsed doHResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("parsing DoH answer for %s: %w", host, err)
		}
		var addrs []netip.Addr
		wantType := 1 // A
		if qtype == "AAAA" {
			wantType = 28
		}
		for _, answer := range parsed.Answer {
			if answer.Type != wantType {
				continue
			}
			if addr, err := netip.ParseAddr(strings.TrimSpace(answer.Data)); err == nil {
				addrs = append(addrs, addr)
			}
		}
		if len(addrs) > 0 {
			return addrs, nil
		}
	}
	return nil, fmt.Errorf("no A or AAAA record for %s", host)
}

// IsBlockedIP reports whether an address must not be dialed: loopback,
// private, link-local, multicast, CGNAT or otherwise non-public space. This
// is the SSRF guard for the DoH answer.
func IsBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	// Reject transition mechanisms conservatively: their embedded IPv4
	// target may be private even when the outer IPv6 address is global.
	for _, prefix := range []netip.Prefix{
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("2002::/16"),
		netip.MustParsePrefix("2001::/32"),
	} {
		if prefix.Contains(ip) {
			return true
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 and IPv4-mapped forms.
	if ip.Is4() || ip.Is4In6() {
		b := ip.Unmap().As4()
		if b[0] == 100 && b[1]&0xC0 == 64 {
			return true
		}
	}
	return !ip.IsGlobalUnicast()
}

// validateURL enforces http(s) scheme and, unless r permits it, ports 80/443 only.
func (r *Resolver) validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	port := u.Port()
	if !r.AllowAnyPort && port != "" && port != "80" && port != "443" {
		return fmt.Errorf("unsupported port %q", port)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

// Follow walks the redirect chain starting at rawURL, resolving every host
// through r.Lookup and dialing the answers directly (TLS SNI and Host still
// use the original host name). No cookies are sent and response bodies are
// discarded after a small read - only the status line and Location matter.
func (r *Resolver) Follow(ctx context.Context, rawURL string) (*Result, error) {
	maxHops := r.MaxHops
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	perHop := r.PerHopTimeout
	if perHop <= 0 {
		perHop = 8 * time.Second
	}

	transport := &http.Transport{
		IdleConnTimeout: 10 * time.Second,
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := r.Lookup(dialCtx, host)
			if err != nil {
				return nil, err
			}
			dialer := &net.Dialer{Timeout: perHop}
			var lastErr error
			for _, ip := range ips {
				if !r.AllowPrivate && IsBlockedIP(ip) {
					lastErr = fmt.Errorf("refusing non-public address %s for %s", ip, host)
					continue
				}
				conn, err := dialer.DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no usable address for %s", host)
			}
			return nil, lastErr
		},
	}
	// Each Follow builds its own transport; release its idle connections and
	// their read goroutines when the walk ends.
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		// Manual redirect handling: no cookie jar, record every hop.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	result := &Result{}
	current := rawURL
	seen := map[string]bool{}
	for hop := 0; hop < maxHops; hop++ {
		parsed, err := url.Parse(current)
		if err != nil {
			return result, fmt.Errorf("hop %d: %w", hop+1, err)
		}
		if err := r.validateURL(parsed); err != nil {
			return result, fmt.Errorf("hop %d (%s): %w", hop+1, current, err)
		}
		if seen[current] {
			return result, fmt.Errorf("redirect loop at %s", current)
		}
		seen[current] = true

		hopCtx, cancel := context.WithTimeout(ctx, perHop)
		req, err := http.NewRequestWithContext(hopCtx, http.MethodGet, current, nil)
		if err != nil {
			cancel()
			return result, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			return result, fmt.Errorf("hop %d (%s): %w", hop+1, current, err)
		}
		// Only the status line and Location matter; discard the body.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()

		result.Hops = append(result.Hops, Hop{URL: current, StatusCode: resp.StatusCode})
		result.FinalURL = current

		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return result, nil
		}
		location := resp.Header.Get("Location")
		if location == "" {
			return result, nil
		}
		next, err := parsed.Parse(location) // resolves relative redirects
		if err != nil {
			return result, fmt.Errorf("hop %d: bad redirect target %q: %w", hop+1, location, err)
		}
		current = next.String()
	}
	return result, fmt.Errorf("redirect chain longer than %d hops", maxHops)
}
