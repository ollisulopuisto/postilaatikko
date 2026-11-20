// Package perflog captures a bounded, private, local performance session.
// It never starts a listener or uploads diagnostics.
package perflog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

const Header = "X-Postilaatikko-Perf-Session"
const Duration = 5 * time.Minute

var validID = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{16}$`)
var global struct {
	sync.Mutex
	session *Session
}

var handshake struct {
	sync.Mutex
	run func()
}

// SetDesktopHandshake registers the read-only helper activation request once
// desktop service initialization has completed. Existing captures also join.
func SetDesktopHandshake(run func()) {
	handshake.Lock()
	handshake.run = run
	handshake.Unlock()
	if Active() != nil {
		requestHelper()
	}
}
func requestHelper() {
	handshake.Lock()
	run := handshake.run
	handshake.Unlock()
	if run != nil {
		go run()
	}
}

type Session struct {
	mu        sync.Mutex
	ID, Dir   string
	file      *os.File
	done      chan struct{}
	closeOnce sync.Once
	cpuStop   chan struct{}
	cpuDone   chan struct{}
}

// Root is stable across releases and separate from mailbox data.
func Root() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "Postilaatikko", "perf"), err
	}
	cache, err := os.UserCacheDir()
	return filepath.Join(cache, "Postilaatikko", "perf"), err
}

func newID() (string, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(data[:]), nil
}

// Start enables one desktop session; normal app behavior is unchanged when unused.
func Start() (*Session, error) {
	if s := Active(); s != nil {
		return s, nil
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	return StartNamed(id, "desktop")
}

// StartNamed joins a validated session using only a local fixed-root path.
func StartNamed(id, process string) (*Session, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	return start(root, id, process, Duration, 60*time.Second)
}
func start(root, id, process string, duration, cpuDuration time.Duration) (*Session, error) {
	if !validID.MatchString(id) || (process != "desktop" && process != "helper") {
		return nil, fmt.Errorf("invalid performance session")
	}
	global.Lock()
	defer global.Unlock()
	if global.session != nil {
		select {
		case <-global.session.done:
		default:
			if global.session.ID != id {
				return nil, fmt.Errorf("another performance session is active")
			}
			return global.session, nil
		}
	}
	dir := filepath.Join(root, id, process)
	if err := privateRoot(root); err != nil {
		return nil, err
	}
	for _, path := range []string{root, filepath.Dir(dir), dir} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe diagnostics directory")
		}
	}
	if err := os.Chmod(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "performance.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	s := &Session{ID: id, Dir: dir, file: file, done: make(chan struct{}), cpuStop: make(chan struct{}), cpuDone: make(chan struct{})}
	global.session = s
	if process == "desktop" {
		requestHelper()
	}
	s.Write(map[string]any{"event": "session_started", "process": process, "duration_seconds": int(duration.Seconds()), "cpu_seconds": int(cpuDuration.Seconds()), "scope": "local Go process; no mailbox bodies, tokens or query strings are logged; profiles remain local", "helper": "requested only over authenticated loopback API; updated helper required"})
	s.snapshot("start")
	cpu, err := s.create("cpu.pprof")
	if err == nil {
		err = pprof.StartCPUProfile(cpu)
	}
	if err != nil {
		if cpu != nil {
			_ = cpu.Close()
		}
		s.Write(map[string]any{"event": "cpu_unavailable", "error": err.Error()})
		close(s.cpuDone)
	} else {
		go func() {
			defer close(s.cpuDone)
			timer := time.NewTimer(cpuDuration)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-s.cpuStop:
			}
			pprof.StopCPUProfile()
			_ = cpu.Close()
			s.Write(map[string]any{"event": "cpu_saved"})
		}()
	}
	go func() {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}

// Reject existing symlink ancestors before creating diagnostics. The user-owned
// cache/log base may be shared, but every newly created directory is private.
func privateRoot(root string) error {
	var paths []string
	for p := filepath.Clean(root); p != filepath.Dir(p); p = filepath.Dir(p) {
		paths = append(paths, p)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		p := paths[i]
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			if err = os.Mkdir(p, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe diagnostics ancestor")
		}
	}
	return os.Chmod(root, 0700)
}
func (s *Session) create(name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(s.Dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
func (s *Session) snapshot(phase string) {
	for _, name := range []string{"heap", "goroutine"} {
		f, err := s.create(name + "-" + phase + ".pprof")
		if err != nil {
			s.Write(map[string]any{"event": "snapshot_error", "profile": name})
			continue
		}
		profile := pprof.Lookup(name)
		if profile != nil {
			err = profile.WriteTo(f, 0)
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			s.Write(map[string]any{"event": "snapshot_error", "profile": name, "phase": phase})
		}
	}
}

// Write records explicit allowlisted diagnostic fields, never request contents.
func (s *Session) Write(fields map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return
	}
	if info, err := s.file.Stat(); err != nil || info.Size() >= 8<<20 {
		return
	}
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	_ = json.NewEncoder(s.file).Encode(fields)
}

// Close finishes profiles before closing the session log. It is idempotent.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.cpuStop)
		<-s.cpuDone
		s.snapshot("end")
		s.Write(map[string]any{"event": "session_finished"})
		s.mu.Lock()
		_ = s.file.Sync()
		_ = s.file.Close()
		s.file = nil
		s.mu.Unlock()
		close(s.done)
	})
}
func Active() *Session {
	global.Lock()
	defer global.Unlock()
	s := global.session
	if s == nil {
		return nil
	}
	select {
	case <-s.done:
		return nil
	default:
		return s
	}
}

// Loopback refuses hostnames and remote URLs. Diagnostic session markers never
// travel to a remote helper or follow a redirect to a different destination.
func Loopback(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Transport records desktop API latency and requests helper diagnostics only
// over the existing authenticated literal-loopback connection.
type Transport struct{ Base http.RoundTripper }

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	s := Active()
	if s == nil {
		return base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Del(Header)
	if req.Response == nil && Loopback(req.URL.Host) && req.Header.Get("Authorization") != "" {
		clone.Header.Set(Header, s.ID)
	}
	started := time.Now()
	resp, err := base.RoundTrip(clone)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	s.Write(map[string]any{"event": "desktop_api_timing", "method": req.Method, "status": status, "duration_ms": time.Since(started).Milliseconds(), "transport_error": err != nil, "local_helper": Loopback(req.URL.Host)})
	if resp != nil && clone.Header.Get(Header) != "" {
		s.Write(map[string]any{"event": "helper_capture_status", "active": resp.Header.Get(Header) == s.ID})
	}
	return resp, err
}

// JoinRequest starts helper capture only after caller authentication and only
// for loopback peer + loopback Host. No path or filename is accepted remotely.
func JoinRequest(r *http.Request) *Session {
	id := r.Header.Get(Header)
	if !validID.MatchString(id) || !Loopback(r.RemoteAddr) || !Loopback(r.Host) {
		return nil
	}
	when, err := time.Parse("20060102T150405Z", id[:16])
	if err != nil || time.Since(when) > Duration || time.Until(when) > 30*time.Second {
		return nil
	}
	s, err := StartNamed(id, "helper")
	if err != nil {
		return nil
	}
	return s
}
