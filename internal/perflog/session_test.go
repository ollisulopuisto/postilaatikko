package perflog

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type trip func(*http.Request) (*http.Response, error)

func (f trip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func testSession(t *testing.T) *Session {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	s, err := start(tempRoot(t), id, "desktop", time.Minute, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}
func TestAutomaticFilesPrivateBoundedAndClosed(t *testing.T) {
	s := testSession(t)
	s.Write(map[string]any{"event": "test"})
	s.Close()
	s.Close()
	for _, name := range []string{"cpu.pprof", "heap-start.pprof", "heap-end.pprof", "goroutine-start.pprof", "goroutine-end.pprof", "performance.jsonl"} {
		path := filepath.Join(s.Dir, name)
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing %s: %v", name, err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal("not private", name, info.Mode())
		}
		if strings.HasSuffix(name, "pprof") {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(name, err)
			}
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Fatal(name, err)
			}
			reader.Close()
			f.Close()
		}
	}
	if Active() != nil {
		t.Fatal("expired session still active")
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir, "performance.jsonl"))
	if !bytes.Contains(b, []byte("session_finished")) {
		t.Fatal("no final status")
	}
}
func TestTransportOnlyMarksAuthenticatedLiteralLoopbackAndRedactsLogs(t *testing.T) {
	s := testSession(t)
	for _, tc := range []struct {
		url, auth string
		mark      bool
	}{{"http://127.0.0.1:8029/v1/accounts/secret?token=notlogged", "Bearer private", true}, {"https://example.test/v1/accounts/private", "Bearer private", false}, {"http://localhost:8029/v1/messages/private", "Bearer private", false}, {"http://127.0.0.1:8029/v1/accounts/private", "", false}} {
		req, _ := http.NewRequest("GET", tc.url, nil)
		req.Header.Set("Authorization", tc.auth)
		_, err := (Transport{Base: trip(func(r *http.Request) (*http.Response, error) {
			if (r.Header.Get(Header) != "") != tc.mark {
				t.Fatal("marker scope", tc.url)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		})}).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		if req.Header.Get(Header) != "" {
			t.Fatal("original mutated")
		}
	}
	s.Close()
	log, _ := os.ReadFile(filepath.Join(s.Dir, "performance.jsonl"))
	for _, secret := range []string{"notlogged", "Bearer private", "example.test", "/v1/accounts"} {
		if bytes.Contains(log, []byte(secret)) {
			t.Fatal("private data logged", secret)
		}
	}
}
func TestRejectUnsafePathsAndRemoteJoin(t *testing.T) {
	for _, id := range []string{"../escape", "anything", "20260101T000000Z-0123456789abcdef/escape"} {
		if _, err := start(tempRoot(t), id, "helper", time.Second, time.Millisecond); err == nil {
			t.Fatal("invalid accepted", id)
		}
	}
	id, _ := newID()
	root := tempRoot(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, id)); err != nil {
		t.Fatal(err)
	}
	if _, err := start(root, id, "helper", time.Second, time.Millisecond); err == nil {
		t.Fatal("symlink accepted")
	}
	for _, remote := range []string{"203.0.113.1:1234", "127.0.0.1:1234"} {
		r, _ := http.NewRequest("GET", "http://remote.example/v1/capabilities", nil)
		r.RemoteAddr = remote
		r.Header.Set(Header, id)
		if JoinRequest(r) != nil {
			t.Fatal("remote join accepted")
		}
	}
}
func TestAutomaticTimeoutAndConcurrentIdempotentStart(t *testing.T) {
	id, _ := newID()
	s, err := start(tempRoot(t), id, "desktop", 20*time.Millisecond, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	same, err := Start()
	if err != nil || same != s {
		t.Fatal("duplicate session", err)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not stop")
	}
}

func TestRedirectHasNoDiagnosticMarker(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	req, _ := http.NewRequest("GET", "http://127.0.0.1:1234/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Response = &http.Response{StatusCode: 302}
	_, err := (Transport{Base: trip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get(Header) != "" {
			t.Fatal("redirect marked")
		}
		return &http.Response{StatusCode: 200}, nil
	})}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
}
func TestRejectSymlinkAncestor(t *testing.T) {
	root := tempRoot(t)
	target := t.TempDir()
	link := filepath.Join(root, "logs")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	id, _ := newID()
	if _, err := start(filepath.Join(link, "perf"), id, "desktop", time.Second, time.Millisecond); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "perf")); !os.IsNotExist(err) {
		t.Fatal("wrote through symlink")
	}
}
func TestHandshakeAfterDesktopReadyAndOnNextSession(t *testing.T) {
	SetDesktopHandshake(nil)
	defer SetDesktopHandshake(nil)
	s := testSession(t)
	defer s.Close()
	ready := make(chan struct{}, 4)
	SetDesktopHandshake(func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("active session not joined after init")
	}
	s.Close()
	id, _ := newID()
	s2, err := start(tempRoot(t), id, "desktop", time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("new session not joined")
	}
}

func TestSessionDirectoryAndLogCap(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	for _, dir := range []string{s.Dir, filepath.Dir(s.Dir), filepath.Dir(filepath.Dir(s.Dir))} {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatal("directory not private", dir, err)
		}
	}
	s.mu.Lock()
	if err := s.file.Truncate(8 << 20); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	s.Write(map[string]any{"event": "must_not_grow"})
	info, err := os.Stat(filepath.Join(s.Dir, "performance.jsonl"))
	if err != nil || info.Size() != 8<<20 {
		t.Fatal("JSON cap exceeded", err)
	}
}
