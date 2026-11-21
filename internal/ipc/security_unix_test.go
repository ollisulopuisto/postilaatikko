//go:build linux || darwin

package ipc

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func TestUnixSocketPermissionsAndContextCleanup(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	tm, err := NewTokenManager()
	if err != nil {
		t.Fatal(err)
	}
	s := NewUnixServer(tm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		info, err := os.Stat(s.Address())
		if err == nil && info.Mode().Perm() == 0600 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket not ready with 0600")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start ignored cancellation")
	}
	if _, err := os.Stat(s.Address()); !os.IsNotExist(err) {
		t.Fatal("socket not removed", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestDelayedIPCStartKeepsCapturedSocketRoot(t *testing.T) {
	first, err := os.MkdirTemp("/tmp", "ipc-first-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(first); err != nil {
			t.Error(err)
		}
	}()
	second, err := os.MkdirTemp("/tmp", "ipc-next-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(second); err != nil {
			t.Error(err)
		}
	}()
	t.Setenv("TMPDIR", first)
	server := NewUnixServer(nil)
	address := server.Address()
	t.Setenv("TMPDIR", second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Start(ctx) }()
	defer func() { cancel(); _ = server.Stop(); <-done }()
	deadline := time.Now().Add(time.Second)
	for {
		conn, err := net.DialTimeout("unix", address, 20*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delayed Start did not bind captured socket", err)
		}
		time.Sleep(time.Millisecond)
	}
	if server.Address() != address {
		t.Fatal("Start retargeted socket", server.Address(), address)
	}
}
