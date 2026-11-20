package automation_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/perflog"
)

func TestAuthenticatedLocalPerformanceSessionWritesDespiteWarnLevel(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	store := automation.NewMemoryTokenStore()
	grant, err := store.RegisterClient("desktop", []v1.Capability{v1.CapabilityMailRead})
	if err != nil {
		t.Fatal(err)
	}
	server := automation.NewServer(automation.ServerConfig{Addr: "127.0.0.1:0", TokenStore: store})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	id := time.Now().UTC().Format("20060102T150405Z") + "-0123456789abcdef"
	send := func(token string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL()+"/v1/capabilities", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(perflog.Header, id)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	if resp := send("invalid"); resp.Header.Get(perflog.Header) != "" || perflog.Active() != nil {
		t.Fatal("unauthenticated profiling")
	}
	limited, err := store.RegisterClient("no-mail-read", []v1.Capability{v1.CapabilityDraftRead})
	if err != nil {
		t.Fatal(err)
	}
	if resp := send(limited.Token); resp.Header.Get(perflog.Header) != "" || perflog.Active() != nil {
		t.Fatal("limited grant started profiling")
	}
	resp := send(grant.Token)
	if resp.Header.Get(perflog.Header) != id {
		t.Fatal("helper did not acknowledge local capture")
	}
	session := perflog.Active()
	if session == nil {
		t.Fatal("no helper session")
	}
	session.Close()
	b, err := os.ReadFile(filepath.Join(session.Dir, "performance.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "helper_api_timing") || !strings.Contains(string(b), "/v1/capabilities") {
		t.Fatal("timing not written", string(b))
	}
	if strings.Contains(string(b), grant.Token) {
		t.Fatal("token leaked")
	}
}

func TestRemoteHostCannotStartPerformanceCapture(t *testing.T) {
	store := automation.NewMemoryTokenStore()
	grant, err := store.RegisterClient("test", []v1.Capability{v1.CapabilityMailRead})
	if err != nil {
		t.Fatal(err)
	}
	server := automation.NewServer(automation.ServerConfig{Addr: "127.0.0.1:0", TokenStore: store, AllowedHosts: []string{"remote.example"}})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	req, _ := http.NewRequest("GET", server.URL()+"/v1/capabilities", nil)
	req.Host = "remote.example"
	req.Header.Set("Authorization", "Bearer "+grant.Token)
	req.Header.Set(perflog.Header, time.Now().UTC().Format("20060102T150405Z")+"-1123456789abcdef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get(perflog.Header) != "" || perflog.Active() != nil {
		t.Fatal("nonloopback host enabled profiling")
	}
}
