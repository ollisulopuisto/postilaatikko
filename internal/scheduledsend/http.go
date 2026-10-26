package scheduledsend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/box"
)

type receipt struct {
	Version     int     `json:"version"`
	RequestHash string  `json:"requestHash"`
	Status      *Status `json:"status,omitempty"`
	ErrorCode   string  `json:"errorCode,omitempty"`
}

// RemoteError contains a fixed authenticated helper error code.
type RemoteError struct{ Code string }

// Error describes a helper rejection without exposing provider error strings.
func (e *RemoteError) Error() string { return "Send Later helper: " + e.Code }

// Handler accepts only authenticated encrypted commands. Pairing, key creation,
// revocation and administrative unknown-outcome resolution are local operations.
// Wrap it in a server with body/header/read/write limits; use TLS off-loopback.
func (s *Store) Handler() http.Handler {
	concurrent := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if request.URL.Path != "/v1/commands" {
			http.NotFound(w, request)
			return
		}
		if request.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		select {
		case concurrent <- struct{}{}:
			defer func() { <-concurrent }()
		default:
			http.Error(w, "helper busy; retry the saved request", http.StatusServiceUnavailable)
			return
		}
		encoded, err := io.ReadAll(http.MaxBytesReader(w, request.Body, MaxEnvelopeBytes))
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		// Authenticate before returning even an encrypted error receipt. This
		// also prevents an unpaired party from probing a paired client's jobs.
		command, clientKey, permissions, err := s.authenticate(request.Context(), encoded)
		if err != nil {
			http.Error(w, "request rejected", http.StatusForbidden)
			return
		}
		if command.Delivery != nil {
			defer clear(command.Delivery.RFC822)
		}
		status, acceptErr := s.acceptCommand(request.Context(), encoded, command, clientKey, permissions)
		response := receipt{Version: ProtocolVersion, RequestHash: requestHash(encoded)}
		if acceptErr == nil {
			response.Status = &status
		}
		if acceptErr != nil {
			response.ErrorCode = receiptErrorCode(acceptErr)
		}
		sealed, err := s.sealReceipt(response, clientKey)
		if err != nil {
			http.Error(w, "response unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(sealed)
	})
}

func requestHash(encoded []byte) string {
	digest := sha256.Sum256(encoded)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func receiptErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrStaleRevision):
		return "stale_revision"
	case errors.Is(err, ErrDeliveryStarted):
		return "delivery_started"
	case errors.Is(err, ErrUnpaired):
		return "not_authorized"
	case errors.Is(err, ErrQueueFull):
		return "queue_full"
	case errors.Is(err, sql.ErrNoRows):
		return "job_not_found"
	default:
		return "request_unconfirmed"
	}
}

func (s *Store) sealReceipt(response receipt, clientKey PublicKey) ([]byte, error) {
	plaintext, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	ciphertext := box.Seal(nil, plaintext, &nonce, (*[32]byte)(&clientKey), &s.helper.private)
	return json.Marshal(Envelope{ProtocolVersion, s.helper.PublicKey().String(), clientKey.String(), nonce[:], ciphertext})
}

// SendEnvelope posts previously persisted encrypted bytes and verifies the
// helper's encrypted durable acknowledgement. Retry the identical bytes after
// network failures; resealing the same revision generates a conflict.
func SendEnvelope(ctx context.Context, endpoint string, encoded []byte, client *Identity, helper PublicKey) (Status, error) {
	var empty Status
	if client == nil || len(encoded) > MaxEnvelopeBytes {
		return empty, errors.New("client identity and bounded encrypted request are required")
	}
	if err := validatePublicKey(helper); err != nil {
		return empty, err
	}
	address, err := commandURL(endpoint)
	if err != nil {
		return empty, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(encoded))
	if err != nil {
		return empty, err
	}
	request.Header.Set("Content-Type", "application/json")
	clientHTTP := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("helper redirects are not allowed")
	}}
	response, err := clientHTTP.Do(request)
	if err != nil {
		return empty, fmt.Errorf("helper acknowledgement unavailable; retry the saved request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return empty, fmt.Errorf("helper returned HTTP %d; no verified acknowledgement", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return empty, errors.New("invalid helper acknowledgement size")
	}
	envelope, err := decodeEnvelope(body)
	if err != nil || envelope.HelperKey != helper.String() || envelope.ClientKey != client.PublicKey().String() {
		return empty, errors.New("helper acknowledgement identity mismatch")
	}
	var nonce [24]byte
	copy(nonce[:], envelope.Nonce)
	plaintext, ok := box.Open(nil, envelope.Ciphertext, &nonce, (*[32]byte)(&helper), &client.private)
	if !ok {
		return empty, errors.New("helper acknowledgement authentication failed")
	}
	defer clear(plaintext)
	var acknowledgement receipt
	if err := decodeJSON(plaintext, &acknowledgement); err != nil || acknowledgement.Version != ProtocolVersion || acknowledgement.RequestHash != requestHash(encoded) {
		return empty, errors.New("helper acknowledgement does not match the submitted request")
	}
	if acknowledgement.ErrorCode != "" {
		return empty, &RemoteError{Code: acknowledgement.ErrorCode}
	}
	if acknowledgement.Status == nil {
		return empty, errors.New("helper acknowledgement has no durable status")
	}
	return *acknowledgement.Status, nil
}

// ValidateEndpoint checks transport security without contacting the helper.
func ValidateEndpoint(endpoint string) error {
	_, err := commandURL(endpoint)
	return err
}

func commandURL(endpoint string) (string, error) {
	address, err := url.Parse(endpoint)
	if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return "", errors.New("invalid helper endpoint")
	}
	loopback := strings.EqualFold(address.Hostname(), "localhost")
	if ip := net.ParseIP(address.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if address.Scheme != "https" && !(address.Scheme == "http" && loopback) {
		return "", errors.New("helper connections require HTTPS except on loopback")
	}
	address.Path = strings.TrimRight(address.Path, "/") + "/v1/commands"
	return address.String(), nil
}
