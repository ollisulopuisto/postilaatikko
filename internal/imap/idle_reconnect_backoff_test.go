package imap

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// idleRejectingServer accepts connections, answers OK to everything except
// IDLE, which it refuses with BAD — the shape of a server/proxy that drops or
// rejects IDLE immediately after a successful login.
type idleRejectingServer struct {
	listener net.Listener
	accepts  atomic.Int32
	mu       sync.Mutex
	acceptAt []time.Time
}

// acceptTimes returns the timestamp of every accepted connection, so tests
// can assert the spacing between retries directly instead of inferring the
// backoff from total elapsed time.
func (s *idleRejectingServer) acceptTimes() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.acceptAt...)
}

func newIdleRejectingServer(t *testing.T) *idleRejectingServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &idleRejectingServer{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.accepts.Add(1)
			s.mu.Lock()
			s.acceptAt = append(s.acceptAt, time.Now())
			s.mu.Unlock()
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = fmt.Fprint(conn, "* OK test IMAP server\r\n")
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					fields := strings.Fields(scanner.Text())
					if len(fields) < 2 {
						continue
					}
					switch {
					case strings.EqualFold(fields[1], "CAPABILITY"):
						_, _ = fmt.Fprint(conn, "* CAPABILITY IMAP4rev1 UIDPLUS IDLE\r\n")
						_, _ = fmt.Fprintf(conn, "%s OK completed\r\n", fields[0])
					case strings.EqualFold(fields[1], "IDLE"):
						_, _ = fmt.Fprintf(conn, "%s BAD IDLE not supported here\r\n", fields[0])
					default:
						_, _ = fmt.Fprintf(conn, "%s OK completed\r\n", fields[0])
					}
				}
			}(conn)
		}
	}()
	return s
}

func (s *idleRejectingServer) credentials(accountID string) (*ClientConfig, error) {
	host, portString, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		return nil, err
	}
	port, err := net.LookupPort("tcp", portString)
	if err != nil {
		return nil, err
	}
	return &ClientConfig{
		Host:           host,
		Port:           port,
		Security:       SecurityNone,
		Username:       accountID,
		Password:       "test",
		ConnectTimeout: time.Second,
		ReadTimeout:    time.Second,
		WriteTimeout:   time.Second,
	}, nil
}

// A fast-failing IDLE cycle used to reconnect with zero backoff forever: the
// loop reset attempts on every successful TCP connect, so a server that
// accepts LOGIN but rejects IDLE pinned a core on reconnect churn. Failed
// cycles must consume reconnect attempts (and backoff) the same way failed
// connects do, until the loop gives up.
func TestIdleCycleFailureAppliesReconnectBackoff(t *testing.T) {
	server := newIdleRejectingServer(t)
	cfg := DefaultIdleConfig()
	cfg.HealthCheckEnabled = false
	cfg.ReconnectBackoff = 20 * time.Millisecond
	cfg.MaxReconnectBackoff = 100 * time.Millisecond
	cfg.MaxReconnectAttempts = 3
	c := newIdleConnection("a", "a", cfg, server.credentials)
	started := time.Now()
	c.Start(context.Background(), make(chan MailEvent, 1))
	select {
	case <-c.doneCh:
	case <-time.After(5 * time.Second):
		c.Stop()
		t.Fatalf("IDLE loop never gave up after %d fast-failing cycles (still reconnecting with no backoff)", cfg.MaxReconnectAttempts)
	}
	// Backoff must actually delay retries: attempts 1 and 2 wait
	// ReconnectBackoff (20ms) and its doubling (40ms) before the third
	// failure exhausts the budget, so giving up takes at least 60ms.
	if elapsed := time.Since(started); elapsed < 60*time.Millisecond {
		t.Fatalf("retries ran without backoff: gave up after %v, want at least 60ms of retry delay", elapsed)
	}
	if got := server.accepts.Load(); got > 10 {
		t.Fatalf("reconnect storm: %d connections accepted for %d allowed attempts", got, cfg.MaxReconnectAttempts)
	}
	// Every retry must wait at least ReconnectBackoff (20ms), and the
	// exponential growth must push the second retry past 30ms — asserted on
	// accept-to-accept spacing, not just total elapsed time, so a regression
	// to immediate retries fails even when the loop still gives up.
	times := server.acceptTimes()
	if len(times) < 3 {
		t.Fatalf("expected at least 3 connection attempts before giving up, got %d", len(times))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 15*time.Millisecond {
			t.Fatalf("retry %d ran without backoff: %v between accepts, want at least ~%v", i, gap, cfg.ReconnectBackoff)
		}
	}
	if gap := times[2].Sub(times[1]); gap < 30*time.Millisecond {
		t.Fatalf("backoff did not grow between retries: second gap %v, want ~%v", gap, 2*cfg.ReconnectBackoff)
	}
}
