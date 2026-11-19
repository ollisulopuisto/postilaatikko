package automation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/hkdb/aerion/internal/automation"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/logging"
)

// withCapturedLog swaps the global logger for one writing JSON to a buffer.
func withCapturedLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := logging.Logger
	logging.Logger = zerolog.New(&buf).Level(zerolog.InfoLevel)
	t.Cleanup(func() { logging.Logger = previous })
	return &buf
}

func startTimingServer(t *testing.T, timing bool) (*automation.Server, *automation.ClientGrant) {
	t.Helper()
	store := automation.NewMemoryTokenStore()
	grant, err := store.RegisterClient("timing-test", []v1.Capability{v1.CapabilityMailRead})
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	server := automation.NewServer(automation.ServerConfig{
		Addr:          "127.0.0.1:0",
		TokenStore:    store,
		RequestTiming: timing,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	})
	return server, grant
}

func getCapabilities(t *testing.T, server *automation.Server, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL()+"/v1/capabilities", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	return resp
}

func timingLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			t.Fatalf("log line is not JSON: %v: %q", err, raw)
		}
		if entry["message"] == "api request timing" {
			lines = append(lines, entry)
		}
	}
	return lines
}

func TestRequestTiming_EnabledLogsTransportMetadata(t *testing.T) {
	buf := withCapturedLog(t)
	server, grant := startTimingServer(t, true)

	resp := getCapabilities(t, server, grant.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	lines := timingLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("timing lines = %d, want 1; buffer: %s", len(lines), buf.String())
	}
	line := lines[0]
	if line["route"] != "/v1/capabilities" {
		t.Errorf("route = %v, want /v1/capabilities", line["route"])
	}
	if line["method"] != http.MethodGet {
		t.Errorf("method = %v, want GET", line["method"])
	}
	if line["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", line["status"])
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
	if _, ok := line["response_bytes"]; !ok {
		t.Error("response_bytes missing")
	}
	if line["request_id"] == "" {
		t.Error("request_id missing")
	}
	// The timing line must never carry credentials or mailbox content.
	for _, key := range []string{"token", "authorization", "subject", "body", "query"} {
		if _, ok := line[key]; ok {
			t.Errorf("timing line must not contain %q", key)
		}
	}
	if strings.Contains(buf.String(), grant.Token) {
		t.Error("log output contains the bearer token")
	}
}

func TestRequestTiming_DisabledByDefault(t *testing.T) {
	buf := withCapturedLog(t)
	server, grant := startTimingServer(t, false)

	resp := getCapabilities(t, server, grant.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if lines := timingLines(t, buf); len(lines) != 0 {
		t.Fatalf("timing lines with timing disabled = %d, want 0", len(lines))
	}
}

func TestRequestTiming_RejectedRequestIsTimed(t *testing.T) {
	buf := withCapturedLog(t)
	server, _ := startTimingServer(t, true)

	req, err := http.NewRequest(http.MethodGet, server.URL()+"/v1/capabilities", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	lines := timingLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("timing lines = %d, want 1", len(lines))
	}
	if lines[0]["status"] != float64(http.StatusUnauthorized) {
		t.Errorf("status = %v, want 401", lines[0]["status"])
	}
}
