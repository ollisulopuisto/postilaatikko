package main

import (
	"bytes"
	"github.com/hkdb/aerion/internal/perflog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOptedInAPITimingIsNotSuppressedByWarnLevel(t *testing.T) {
	t.Setenv("ETERNOMAIL_API_TIMING", "")
	if got := helperLogLevel(); got != "warn" {
		t.Fatal(got)
	}
	t.Setenv("ETERNOMAIL_API_TIMING", "1")
	if got := helperLogLevel(); got != "info" {
		t.Fatal(got)
	}
}

func TestShutdownFinalizesCaptureStartedDuringServiceOperation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	// Register shutdown before the capture exists, matching service lifetime.
	stopped := make(chan struct{})
	exit := make(chan struct{})
	go func() { defer close(stopped); defer finishPerformanceCapture(); <-exit }()
	req, _ := http.NewRequest("GET", "http://127.0.0.1:8029/v1/capabilities", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(perflog.Header, time.Now().UTC().Format("20060102T150405Z")+"-c123456789abcdef")
	session := perflog.JoinRequest(req)
	if session == nil {
		t.Fatal("helper capture not started")
	}
	close(exit)
	<-stopped
	if perflog.Active() != nil {
		t.Fatal("shutdown left capture active")
	}
	for _, name := range []string{"cpu.pprof", "heap-end.pprof", "goroutine-end.pprof", "performance.jsonl"} {
		b, err := os.ReadFile(filepath.Join(session.Dir, name))
		if err != nil || len(b) == 0 {
			t.Fatal("incomplete profile", name, err)
		}
		if name == "performance.jsonl" && !bytes.Contains(b, []byte("session_finished")) {
			t.Fatal("no completion")
		}
	}
}
