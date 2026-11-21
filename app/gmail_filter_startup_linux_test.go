//go:build linux

package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/automation"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/ipc"
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
			// Unix IPC uses os.TempDir independently of XDG. Keep it short enough
			// to avoid the socket implementation's long-path fallback to shared /tmp.
			socketDir, err := os.MkdirTemp("/tmp", "pl-start-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(socketDir); err != nil {
					t.Error(err)
				}
			})
			t.Setenv("TMPDIR", socketDir)
			if !strings.HasPrefix(ipc.NewUnixServer(nil).Address(), socketDir+string(os.PathSeparator)) {
				t.Fatal("startup would use a shared IPC socket")
			}

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
				defer func() {
					if err := competing.Release(); err != nil {
						t.Error(err)
					}
				}()
			}
			sharedTransport := http.DefaultTransport
			var helperCalls atomic.Int32
			if helperOwns {
				// Preserve the startup-provisioned client's endpoint, token and
				// transport. Redirect only its actual socket request to an
				// isolated helper running the production auth middleware.
				helper := automation.NewServer(automation.ServerConfig{Addr: "127.0.0.1:0", TokenStore: automation.NewSQLiteTokenStore(a.db), V1Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1/filters/gmail" {
						http.NotFound(w, r)
						return
					}
					if !automation.RequireGrantCapability(w, r, v1.CapabilityMailRead) {
						return
					}
					helperCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"data":{"accountId":"helper-mail","email":"helper@example.test","consentRequired":true}}`)
				})})
				if err := helper.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := helper.Stop(context.Background()); err != nil {
						t.Error(err)
					}
				}()
				routedTransport := http.DefaultTransport.(*http.Transport).Clone()
				routedTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if network != "tcp" || address != "127.0.0.1:8029" {
						return nil, fmt.Errorf("unexpected helper destination: %s %s", network, address)
					}
					return (&net.Dialer{}).DialContext(ctx, network, helper.Addr())
				}
				a.mailboxClientTransport = routedTransport
				defer routedTransport.CloseIdleConnections()
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
			defer func() { cancel(); a.Shutdown(ctx) }()
			if http.DefaultTransport != sharedTransport {
				t.Fatal("startup fixture mutated shared HTTP transport")
			}

			// Address is precomputed. A real connection proves Start has bound while
			// TMPDIR remains isolated; Shutdown then joins the server before restore.
			deadline := time.Now().Add(time.Second)
			for {
				conn, err := net.DialTimeout("unix", a.ipcServer.Address(), 20*time.Millisecond)
				if err == nil {
					_ = conn.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("isolated IPC never bound", err)
				}
				time.Sleep(time.Millisecond)
			}

			if !reachedReady || a.mailboxService == nil || a.filterStore == nil {
				t.Fatal("production startup path was not exercised")
			}
			if a.ipcServer == nil || !strings.HasPrefix(a.ipcServer.Address(), socketDir+string(os.PathSeparator)) {
				t.Fatal("startup used a shared IPC socket")
			}
			if !helperOwns {
				client, workspace, err := a.captureNativeGmailWorkspace()
				if err != nil || client != nil || workspace.Target != "local" || workspace.Key != "standalone" {
					t.Fatalf("started desktop routed away from its executor: %+v %v", workspace, err)
				}
			}

			if helperOwns {
				if a.mailboxService.isRemote() {
					t.Fatal("startup fixture did not use default localhost helper")
				}
				provisioned := a.mailboxService.client
				if provisioned == nil || a.mailboxService.credential == nil {
					t.Fatal("startup did not provision client")
				}

				workspace, err := a.GetNativeGmailWorkspace()
				if err != nil || workspace.Target != "helper" || workspace.Endpoint != mailboxServiceEndpoint {
					t.Fatal("competing localhost owner not selected", workspace, err)
				}
				reply, err := a.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "context", RuleID: "source"})
				if err != nil || reply.Context == nil || reply.Context.AccountID != "helper-mail" || helperCalls.Load() != 1 || a.mailboxService.client != provisioned {
					t.Fatal("startup helper route fell back", reply, err, helperCalls.Load())
				}
			}
			err = a.requireStandaloneFilterOwner()
			if (err == nil) == helperOwns {
				t.Fatalf("helperOwns=%t ownership error=%v", helperOwns, err)
			}
		})
	}
}
