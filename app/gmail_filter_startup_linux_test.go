//go:build linux

package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ollisulopuisto/postilaatikko/internal/syncowner"
	keyring "github.com/zalando/go-keyring"
)

// Runs Preflight and Startup, including the connector/store initialization and
// real mailbox ownership acquisition. Only the native UI-ready notification is
// intercepted, after the ownership path and before optional desktop integrations.
func TestGmailPublicationUsesRealStartupOwnership(t *testing.T) {
	for _, helperOwns := range []bool{false, true} {
		name := "desktop"
		if helperOwns {
			name = "helper"
		}
		t.Run(name, func(t *testing.T) {
			installPreflightTestHelper(t)
			dir := t.TempDir()
			for _, name := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
				t.Setenv(name, filepath.Join(dir, name))
			}
			keyring.MockInit()
			a := &App{}
			if err := a.Preflight(); err != nil {
				t.Fatal(err)
			}
			var competing *syncowner.Lock
			if helperOwns {
				var err error
				competing, err = syncowner.TryAcquire(filepath.Join(a.paths.Data, "mail-sync.owner.lock"))
				if err != nil {
					t.Fatal(err)
				}
				defer competing.Release()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.runtimeEventSubscriber = func(string, func(...interface{})) func() { return func() {} }
			reachedReady := false
			marker := new(int)
			a.runtimeEventEmitter = func(name string, _ ...interface{}) {
				if name == "app:ready" {
					reachedReady = true
					panic(marker)
				}
			}
			func() {
				defer func() {
					if r := recover(); r != nil && r != marker {
						panic(r)
					}
				}()
				a.Startup(ctx)
			}()
			cancel()
			defer a.Shutdown(ctx)
			if !reachedReady || a.mailboxService == nil || a.filterStore == nil {
				t.Fatal("production startup path was not exercised")
			}
			err := a.requireStandaloneFilterOwner()
			if (err == nil) == helperOwns {
				t.Fatalf("helperOwns=%t ownership error=%v", helperOwns, err)
			}
		})
	}
}
