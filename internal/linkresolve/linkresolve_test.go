package linkresolve

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func localLookup(mapping map[string]string) LookupFunc {
	return func(ctx context.Context, host string) ([]netip.Addr, error) {
		target, ok := mapping[host]
		if !ok {
			return nil, &net.DNSError{IsNotFound: true, Name: host}
		}
		addr, err := netip.ParseAddr(target)
		if err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}
}

func TestFollowRedirectChain(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	middle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/landing", http.StatusFound)
	}))
	defer middle.Close()

	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, middle.URL+"/step2", http.StatusMovedPermanently)
	}))
	defer start.Close()

	resolver := &Resolver{
		Lookup:       localLookup(map[string]string{"127.0.0.1": "127.0.0.1"}),
		AllowPrivate: true,
		AllowAnyPort: true,
	}
	result, err := resolver.Follow(context.Background(), start.URL+"/step1")
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if len(result.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %d: %+v", len(result.Hops), result.Hops)
	}
	if result.Hops[0].StatusCode != http.StatusMovedPermanently ||
		result.Hops[1].StatusCode != http.StatusFound ||
		result.Hops[2].StatusCode != http.StatusOK {
		t.Fatalf("unexpected statuses: %+v", result.Hops)
	}
	if result.FinalURL != final.URL+"/landing" {
		t.Fatalf("final URL %q, want %q", result.FinalURL, final.URL+"/landing")
	}
}

func TestFollowCapsHops(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		http.Redirect(w, r, fmt.Sprintf("/next-%d", count), http.StatusFound)
	}))
	defer server.Close()

	resolver := &Resolver{
		Lookup:       localLookup(map[string]string{"127.0.0.1": "127.0.0.1"}),
		AllowPrivate: true,
		AllowAnyPort: true,
		MaxHops:      3,
	}
	result, err := resolver.Follow(context.Background(), server.URL+"/start")
	if err == nil {
		t.Fatal("expected hop limit error")
	}
	if len(result.Hops) != 3 {
		t.Fatalf("expected 3 recorded hops before the error, got %d", len(result.Hops))
	}
}

func TestFollowRejectsBadScheme(t *testing.T) {
	resolver := &Resolver{Lookup: localLookup(nil), AllowPrivate: true}
	if _, err := resolver.Follow(context.Background(), "ftp://example.com/x"); err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestFollowRejectsNonStandardPort(t *testing.T) {
	resolver := &Resolver{Lookup: localLookup(nil), AllowPrivate: true}
	if _, err := resolver.Follow(context.Background(), "https://example.com:8443/x"); err == nil {
		t.Fatal("expected port error")
	}
}

func TestFollowRefusesPrivateAddress(t *testing.T) {
	resolver := &Resolver{
		Lookup:       localLookup(map[string]string{"internal.example": "192.168.1.1"}),
		AllowPrivate: false,
	}
	_, err := resolver.Follow(context.Background(), "http://internal.example/x")
	if err == nil {
		t.Fatal("expected SSRF guard to refuse private address")
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.1",
		"169.254.1.1", "::1", "fe80::1", "0.0.0.0",
		"100.64.0.1", "100.127.255.255", "224.0.0.1",
		// IPv4-mapped forms must not smuggle private targets past the guard.
		"::ffff:127.0.0.1", "::ffff:10.0.0.5", "::ffff:192.168.1.1",
		"::ffff:169.254.169.254", "::ffff:100.64.0.1",
		"64:ff9b::7f00:1", "64:ff9b:1::a00:1", "2002:7f00:1::", "2001::1",
	}
	for _, s := range blocked {
		if !IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "100.63.255.255", "100.128.0.1"}
	for _, s := range allowed {
		if IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestResolveDoHParsesAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") == "A" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Status": 0,
				"Answer": []map[string]any{
					{"type": 5, "data": "alias.example."},
					{"type": 1, "data": "93.184.216.34"},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Status": 3})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := ResolveDoH(ctx, server.URL, "example.com")
	if err != nil {
		t.Fatalf("ResolveDoH: %v", err)
	}
	if len(addrs) != 1 || addrs[0].String() != "93.184.216.34" {
		t.Fatalf("unexpected addrs: %v", addrs)
	}
}
