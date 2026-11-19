package imap

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
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
	c.Start(context.Background(), make(chan MailEvent, 1))
	select {
	case <-c.doneCh:
	case <-time.After(5 * time.Second):
		c.Stop()
		t.Fatalf("IDLE loop never gave up after %d fast-failing cycles (still reconnecting with no backoff)", cfg.MaxReconnectAttempts)
	}
	if got := server.accepts.Load(); got > 10 {
		t.Fatalf("reconnect storm: %d connections accepted for %d allowed attempts", got, cfg.MaxReconnectAttempts)
	}
}
