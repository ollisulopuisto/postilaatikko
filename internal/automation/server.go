package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/automation/v1"
	"github.com/rs/zerolog"
)

const (
	defaultAddress   = "127.0.0.1:0"
	defaultBodyLimit = 1 << 20
	maxHeaderBytes   = 16 << 10
)

// RouteHandler handles an authenticated request after its capability is
// checked. The request ID is safe to return to the caller for log correlation.
type RouteHandler func(http.ResponseWriter, *http.Request, Client, string)

type route struct {
	method     string
	capability v1.Capability
	handler    RouteHandler
}

// ServerConfig configures the local HTTP listener. An empty address uses an
// ephemeral port on IPv4 loopback so the process never binds a LAN interface.
type ServerConfig struct {
	Address      string
	Registry     *Registry
	Logger       zerolog.Logger
	MaxBodyBytes int64
}

// Server is an opt-in, loopback-only HTTP server for authenticated local
// automation clients.
type Server struct {
	mu         sync.RWMutex
	config     ServerConfig
	routes     map[string]route
	listener   net.Listener
	httpServer *http.Server
	started    bool
	stopped    bool
}

// NewServer creates a stopped local API server. The caller must explicitly
// call Start; construction never opens a network listener.
func NewServer(config ServerConfig) (*Server, error) {
	if config.Registry == nil {
		return nil, errors.New("automation: client registry is required")
	}
	if config.Address == "" {
		config.Address = defaultAddress
	}
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = defaultBodyLimit
	}
	if err := validateLoopbackAddress(config.Address); err != nil {
		return nil, err
	}
	s := &Server{config: config, routes: make(map[string]route)}
	if err := s.Register(http.MethodGet, "/v1/capabilities", "", s.handleCapabilities); err != nil {
		return nil, err
	}
	return s, nil
}

// Register adds one exact v1 route and its independently enforced capability.
// Routes may be registered until Stop. Lookup is synchronized so dependent
// endpoint packages can attach their routes after the listener starts.
func (s *Server) Register(method, path string, capability v1.Capability, handler RouteHandler) error {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != http.MethodGet && method != http.MethodPost {
		return fmt.Errorf("automation: unsupported HTTP method %q", method)
	}
	if !strings.HasPrefix(path, "/v1/") || strings.Contains(path, "?") || strings.Contains(path, "#") {
		return errors.New("automation: route must be an exact /v1/ path")
	}
	if handler == nil {
		return errors.New("automation: route handler is required")
	}
	if capability != "" && !capability.Known() {
		return fmt.Errorf("automation: unknown route capability %q", capability)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("automation: routes cannot change after server stop")
	}
	key := method + " " + path
	if _, exists := s.routes[key]; exists {
		return fmt.Errorf("automation: route %s already registered", key)
	}
	s.routes[key] = route{method: method, capability: capability, handler: handler}
	return nil
}

// Start binds the already-validated loopback address and serves until stopped.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("automation: server already started")
	}
	if s.stopped {
		s.mu.Unlock()
		return errors.New("automation: server has been stopped")
	}
	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("automation: bind local API listener: %w", err)
	}
	if !isLoopbackHost(listener.Addr().String()) {
		listener.Close()
		s.mu.Unlock()
		return errors.New("automation: listener did not bind to loopback")
	}
	s.listener = listener
	s.httpServer = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	s.started = true
	server := s.httpServer
	addr := listener.Addr().String()
	s.mu.Unlock()

	s.config.Logger.Info().Str("address", addr).Msg("Local automation API listening on loopback")
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.config.Logger.Error().Err(err).Msg("Local automation API stopped unexpectedly")
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Stop(shutdownCtx); err != nil {
			s.config.Logger.Warn().Err(err).Msg("Local automation API shutdown was not clean")
		}
	}()
	return nil
}

// Stop closes the listener and waits briefly for active requests to finish.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	server := s.httpServer
	listener := s.listener
	s.mu.Unlock()
	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			return fmt.Errorf("automation: stop local API server: %w", err)
		}
		return nil
	}
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("automation: close local API listener: %w", err)
		}
	}
	return nil
}

// Address returns the bound URL after Start, or an empty string before Start.
func (s *Server) Address() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return ""
	}
	return "http://" + s.listener.Addr().String()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.NewString()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Request-ID", requestID)
	if !s.validHost(r) || !remoteIsLoopback(r.RemoteAddr) {
		s.writeError(w, http.StatusForbidden, v1.NewFault(v1.CodeUnauthenticated, "This local API request is not authorized.", false, nil), requestID)
		return
	}
	if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Sec-Fetch-Mode") != "" || r.Method == http.MethodOptions {
		s.writeError(w, http.StatusForbidden, v1.NewFault(v1.CodeUnauthenticated, "Browser-origin requests are not accepted.", false, nil), requestID)
		return
	}
	if r.URL.Query().Has("token") || r.URL.Query().Has("access_token") {
		s.writeError(w, http.StatusBadRequest, v1.InvalidRequest("Credentials must be sent in the Authorization header.", nil), requestID)
		return
	}
	s.mu.RLock()
	registeredRoute, ok := s.routes[r.Method+" "+r.URL.Path]
	s.mu.RUnlock()
	if !ok {
		s.writeError(w, http.StatusNotFound, v1.NewFault(v1.CodeNotFound, "The requested API route was not found.", false, nil), requestID)
		return
	}
	client, ok := s.authenticateRequest(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, v1.NewFault(v1.CodeUnauthenticated, "A valid client credential is required.", false, nil), requestID)
		return
	}
	if registeredRoute.capability != "" && !hasCapability(client.Capabilities, registeredRoute.capability) {
		s.writeError(w, http.StatusForbidden, v1.NewFault(v1.CodeCapabilityDenied, "This client is not authorized for this operation.", false, nil), requestID)
		return
	}
	if err := s.boundRequestBody(w, r); err != nil {
		fault := v1.NewFault(v1.CodeBodyTooLarge, "The request body exceeds the local API size limit.", false, err)
		s.writeError(w, http.StatusRequestEntityTooLarge, fault, requestID)
		return
	}
	registeredRoute.handler(w, r, client, requestID)
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request, client Client, requestID string) {
	capabilities := append([]v1.Capability(nil), client.Capabilities...)
	s.writeJSON(w, http.StatusOK, v1.Response[v1.Capabilities]{
		Data:      v1.Capabilities{Version: v1.Version, Capabilities: capabilities},
		RequestID: requestID,
	})
}

func (s *Server) authenticateRequest(r *http.Request) (Client, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.Count(header, " ") != 1 {
		return Client{}, false
	}
	return s.config.Registry.Authenticate(strings.TrimPrefix(header, "Bearer "))
}

func (s *Server) boundRequestBody(w http.ResponseWriter, r *http.Request) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	if r.ContentLength > s.config.MaxBodyBytes {
		return errors.New("request body exceeds configured limit")
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func (s *Server) validHost(r *http.Request) bool {
	s.mu.RLock()
	listener := s.listener
	s.mu.RUnlock()
	if listener == nil {
		return false
	}
	return strings.EqualFold(r.Host, listener.Addr().String())
}

func (s *Server) writeError(w http.ResponseWriter, status int, fault error, requestID string) {
	var apiFault *v1.Fault
	if !errors.As(fault, &apiFault) {
		apiFault = v1.NewFault(v1.CodeInternal, "The request could not be completed.", false, fault)
	}
	s.config.Logger.Debug().Str("requestId", requestID).Str("code", string(apiFault.Code)).Msg("Local automation API rejected request")
	s.writeJSON(w, status, apiFault.Response(requestID))
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.config.Logger.Debug().Err(err).Msg("Could not write local automation API response")
	}
}

func validateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("automation: invalid listener address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("automation: API listener must bind to a loopback IP address")
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 0 || parsedPort > 65535 {
		return errors.New("automation: invalid listener port")
	}
	return nil
}

func isLoopbackHost(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func remoteIsLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hasCapability(grants []v1.Capability, required v1.Capability) bool {
	for _, grant := range grants {
		if grant == required {
			return true
		}
	}
	return false
}
