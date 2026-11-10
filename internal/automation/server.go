package automation

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/logging"
)

const (
	// MaxRequestBodyBytes bounds the maximum allowable incoming request body size (1 MiB).
	MaxRequestBodyBytes = 1 << 20
)

type contextKey string

const (
	clientGrantKey contextKey = "automation.grant"
	requestIDKey   contextKey = "automation.requestId"
)

// ServerConfig configures the authenticated automation listener.
type ServerConfig struct {
	Addr string
	// TLSCertFile and TLSKeyFile enable TLS; both must be set together.
	TLSCertFile string
	TLSKeyFile  string
	// AllowedHosts contains exact Host names accepted by the listener.
	AllowedHosts []string
	TokenStore   TokenStore
	V1Handler    http.Handler
}

// Server hosts the authenticated automation API.
type Server struct {
	config     ServerConfig
	tlsConfig  *tls.Config
	httpServer *http.Server
	listener   net.Listener
	mu         sync.Mutex
	running    bool
}

// NewServer initializes a local automation HTTP server.
func NewServer(config ServerConfig) *Server {
	if config.Addr == "" {
		config.Addr = "127.0.0.1:0" // Dynamic port on loopback
	}
	if config.TokenStore == nil {
		config.TokenStore = NewMemoryTokenStore()
	}
	config.AllowedHosts = append([]string(nil), config.AllowedHosts...)

	s := &Server{
		config: config,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/capabilities", s.handleCapabilities)
	if config.V1Handler != nil {
		mux.Handle("/v1/", config.V1Handler)
	}

	handler := s.wrapMiddleware(mux)

	s.httpServer = &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start binds to the configured address and starts listening in a background goroutine.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("automation: server is already running")
	}
	tlsConfig, err := s.listenerTLSConfig()
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("automation listen: %w", err)
	}

	if !isLoopbackListener(ln.Addr()) && tlsConfig == nil {
		ln.Close()
		return fmt.Errorf("automation: non-loopback listener requires TLS")
	}
	if !isLoopbackListener(ln.Addr()) && len(s.config.AllowedHosts) == 0 {
		ln.Close()
		return errors.New("automation: non-loopback listener requires an explicit Host allowlist")
	}

	s.listener = ln
	s.tlsConfig = tlsConfig
	s.running = true

	go func() {
		serveListener := ln
		if tlsConfig != nil {
			serveListener = tls.NewListener(ln, tlsConfig)
		}
		if err := s.httpServer.Serve(serveListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log := logging.WithComponent("automation")
			log.Error().Err(err).Msg("automation server error")
		}
	}()

	return nil
}

// Addr returns the bound listener address.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// URL returns the base URL for accessing the server.
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	addr := s.listener.Addr().String()
	if s.tlsConfig != nil {
		return "https://" + addr
	}
	return "http://" + addr
}

func (s *Server) listenerTLSConfig() (*tls.Config, error) {
	certFile := strings.TrimSpace(s.config.TLSCertFile)
	keyFile := strings.TrimSpace(s.config.TLSKeyFile)
	if certFile == "" && keyFile == "" {
		if isRemoteBindAddress(s.config.Addr) {
			return nil, errors.New("automation: non-loopback listener requires TLS certificate and key files")
		}
		return nil, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("automation: both TLS certificate and key files are required")
	}
	if isRemoteBindAddress(s.config.Addr) && !hasAllowedHosts(s.config.AllowedHosts) {
		return nil, errors.New("automation: non-loopback listener requires an explicit Host allowlist")
	}
	if len(s.config.AllowedHosts) > 0 && !hasAllowedHosts(s.config.AllowedHosts) {
		return nil, errors.New("automation: Host allowlist must contain at least one valid host")
	}
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("automation: load TLS certificate: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, nil
}

func hasAllowedHosts(hosts []string) bool {
	for _, host := range hosts {
		host = normalizeHost(host)
		if host != "" && !strings.ContainsAny(host, "* /\\") {
			return true
		}
	}
	return false
}

func isRemoteBindAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip == nil || !ip.IsLoopback()
}

func isLoopbackListener(addr net.Addr) bool {
	tcpAddr, ok := addr.(*net.TCPAddr)
	return ok && tcpAddr.IP.IsLoopback()
}

func (s *Server) allowedHost(host string) bool {
	host = normalizeHost(host)
	if host == "" {
		return false
	}
	if len(s.config.AllowedHosts) == 0 {
		return host == "127.0.0.1" || host == "localhost" || host == "::1"
	}
	for _, allowed := range s.config.AllowedHosts {
		if host == normalizeHost(allowed) {
			return true
		}
	}
	return false
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	host = strings.Trim(host, "[]")
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// Stop shuts down the server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false
	return s.httpServer.Shutdown(ctx)
}

// wrapMiddleware enforces origin protection, host validation, bearer auth, body limiting, and request correlation.
func (s *Server) wrapMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		w.Header().Set("X-Request-Id", reqID)

		// Defense-in-depth HTTP security headers: prevent MIME sniffing, frame embedding, content injection, and sensitive data caching
		applySecurityHeaders(w)

		// 1. Origin Protection: reject any browser Origin header (no CORS allowed)
		if origin := r.Header.Get("Origin"); origin != "" {
			writeError(w, reqID, http.StatusForbidden, v1.CodeCapabilityDenied, "Browser origins are not permitted", false)
			return
		}

		// 2. Host Validation: reject unexpected Host headers to prevent DNS rebinding attacks
		if !s.allowedHost(r.Host) {
			writeError(w, reqID, http.StatusBadRequest, v1.CodeInvalidRequest, "Invalid Host header", false)
			return
		}

		// 3. Reject credentials in URLs or query strings
		if r.URL.User != nil || r.URL.Query().Get("token") != "" || r.URL.Query().Get("access_token") != "" || r.URL.Query().Get("authorization") != "" {
			writeError(w, reqID, http.StatusBadRequest, v1.CodeInvalidRequest, "Credentials in URLs or query strings are strictly prohibited", false)
			return
		}

		// 4. Authenticate Bearer token
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Missing or invalid Authorization header", false)
			return
		}
		token := strings.TrimSpace(authHeader[7:])
		if token == "" {
			writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Bearer token is empty", false)
			return
		}

		grant, err := s.config.TokenStore.GetGrant(token)
		if err != nil {
			if errors.Is(err, ErrTokenRevoked) {
				writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Bearer token has been revoked", false)
				return
			}
			writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Invalid bearer token", false)
			return
		}

		// 5. Enforce MaxRequestBodyBytes
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)

		// 6. Inject grant & request ID into context
		ctx := context.WithValue(r.Context(), clientGrantKey, grant)
		ctx = context.WithValue(ctx, requestIDKey, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleCapabilities handles GET /v1/capabilities.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		reqID := GetRequestID(r.Context())
		writeError(w, reqID, http.StatusMethodNotAllowed, v1.CodeInvalidRequest, "Method not allowed", false)
		return
	}

	grant := GetClientGrant(r.Context())
	if grant == nil {
		reqID := GetRequestID(r.Context())
		writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Not authenticated", false)
		return
	}

	resp := v1.Response[v1.Capabilities]{
		Data: v1.Capabilities{
			Version:      v1.Version,
			Capabilities: grant.Capabilities,
		},
		RequestID: GetRequestID(r.Context()),
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetClientGrant retrieves the authorized client grant from the request context.
func GetClientGrant(ctx context.Context) *ClientGrant {
	g, _ := ctx.Value(clientGrantKey).(*ClientGrant)
	return g
}

// GetRequestID retrieves the request ID from the request context.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	if id == "" {
		return "req-unknown"
	}
	return id
}

// RequireGrantCapability checks if the caller has the required capability, writing a 403 response if missing.
func RequireGrantCapability(w http.ResponseWriter, r *http.Request, cap v1.Capability) bool {
	grant := GetClientGrant(r.Context())
	reqID := GetRequestID(r.Context())
	if grant == nil {
		writeError(w, reqID, http.StatusUnauthorized, v1.CodeUnauthenticated, "Not authenticated", false)
		return false
	}
	for _, c := range grant.Capabilities {
		if c == cap {
			return true
		}
	}
	msg := fmt.Sprintf("Client lacks required capability: %s", cap)
	writeError(w, reqID, http.StatusForbidden, v1.CodeCapabilityDenied, msg, false)
	return false
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, reqID string, httpStatus int, code v1.ErrorCode, message string, retryable bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	resp := v1.ErrorResponse{
		Error: v1.ErrorBody{
			Code:      code,
			Message:   message,
			Retryable: retryable,
			RequestID: reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func applySecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.Set("Cache-Control", "no-store")
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}
