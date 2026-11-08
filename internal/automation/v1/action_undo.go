package v1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrActionUndoUnavailable = errors.New("action is not undoable")
	ErrActionUndoBusy        = errors.New("action undo is already in progress")
	ErrActionUndoConflict    = errors.New("undo idempotency key conflict")
)

// ActionUndoSummary is a privacy-minimal mailbox operation history item.
type ActionUndoSummary struct {
	OperationID   string     `json:"operationId"`
	Action        ActionKind `json:"action"`
	CreatedAt     string     `json:"createdAt"`
	UndoExpiresAt string     `json:"undoExpiresAt,omitempty"`
	MessageCount  int        `json:"messageCount"`
	State         string     `json:"state"`
	UndoState     string     `json:"undoState"`
	UndoAvailable bool       `json:"undoAvailable"`
}

// ActionUndoRecord contains private server-side snapshot data used only to
// execute an undo; it is never returned directly from the API.
type ActionUndoRecord struct {
	OperationID  string
	Action       ActionKind
	State        OperationState
	UndoState    string
	CreatedAt    string
	ExpiresAt    string
	MessageCount int
	Payload      []byte
}

// ActionUndoResult reports the durable result of an Undo request.
type ActionUndoResult struct {
	OperationID string               `json:"operationId"`
	Action      ActionKind           `json:"action"`
	State       string               `json:"state"`
	Affected    []AffectedMessageRef `json:"affected,omitempty"`
	Replayed    bool                 `json:"replayed,omitempty"`
}

// ActionUndoAffectedRefReader reads current mailbox references after Undo.
type ActionUndoAffectedRefReader interface {
	ActionUndoAffectedRefs(context.Context, []byte) ([]AffectedMessageRef, error)
}

// ActionUndoStore owns durable action history and atomic undo claims.
type ActionUndoStore interface {
	ListUndoHistory(context.Context, int) ([]ActionUndoSummary, error)
	ClaimUndo(context.Context, string, string, string) (ActionUndoRecord, error)
	CompleteUndo(context.Context, string, string) error
}

// ActionUndoOperations applies a server-owned inverse operation.
type ActionUndoOperations interface {
	UndoAction(context.Context, ActionKind, []byte) error
}

// MountActionUndoRoutes mounts authenticated durable undo endpoints.
func MountActionUndoRoutes(mux *http.ServeMux, store ActionUndoStore, operations ActionUndoOperations, keyDigest func(string) string, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	if keyDigest == nil {
		keyDigest = func(key string) string {
			digest := sha256.Sum256([]byte(key))
			return hex.EncodeToString(digest[:])
		}
	}
	mux.HandleFunc("/v1/undo/history", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Limit must be between 1 and 100", false)
				return
			}
			limit = parsed
		}
		items, err := store.ListUndoHistory(r.Context(), limit)
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not load undo history", true)
			return
		}
		writeUndoResponse(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("/v1/undo/latest", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		handleActionUndo(w, r, "", store, operations, keyDigest)
	})
	mux.HandleFunc("/v1/undo/operations/", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		operationID := strings.TrimPrefix(r.URL.Path, "/v1/undo/operations/")
		if operationID == "" || strings.Contains(operationID, "/") {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Operation ID is required", false)
			return
		}
		handleActionUndo(w, r, operationID, store, operations, keyDigest)
	})
}

func handleActionUndo(w http.ResponseWriter, r *http.Request, operationID string, store ActionUndoStore, operations ActionUndoOperations, keyDigest func(string) string) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Idempotency-Key header is required for undo", false)
		return
	}
	requestHash := sha256.Sum256([]byte(operationID))
	record, err := store.ClaimUndo(r.Context(), operationID, keyDigest(idempotencyKey), hex.EncodeToString(requestHash[:]))
	if errors.Is(err, ErrActionUndoConflict) {
		writeHeaderError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different undo request", false)
		return
	}
	if errors.Is(err, ErrActionUndoUnavailable) {
		writeHeaderError(w, r, http.StatusConflict, CodeInvalidRequest, "Action is not currently undoable", false)
		return
	}
	if errors.Is(err, ErrActionUndoBusy) {
		writeHeaderError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Action undo is already in progress", true)
		return
	}
	if err != nil {
		writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not claim action undo", true)
		return
	}
	if record.UndoState == "undone" || record.UndoState == "outcome_unknown" {
		result := ActionUndoResult{OperationID: record.OperationID, Action: record.Action, State: record.UndoState, Replayed: true}
		if record.UndoState == "undone" {
			if reader, ok := operations.(ActionUndoAffectedRefReader); ok {
				result.Affected, _ = reader.ActionUndoAffectedRefs(r.Context(), record.Payload)
			}
		}
		writeUndoResponse(w, r, http.StatusOK, result)
		return
	}
	if err := operations.UndoAction(r.Context(), record.Action, record.Payload); err != nil {
		if completeErr := completeActionUndo(store, record.OperationID, "outcome_unknown"); completeErr != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Undo outcome is uncertain and its receipt could not be saved", true)
			return
		}
		writeUndoResponse(w, r, http.StatusOK, ActionUndoResult{OperationID: record.OperationID, Action: record.Action, State: "outcome_unknown"})
		return
	}
	if err := completeActionUndo(store, record.OperationID, "undone"); err != nil {
		writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Undo completed but its receipt could not be saved", true)
		return
	}
	result := ActionUndoResult{OperationID: record.OperationID, Action: record.Action, State: "undone"}
	if reader, ok := operations.(ActionUndoAffectedRefReader); ok {
		result.Affected, _ = reader.ActionUndoAffectedRefs(r.Context(), record.Payload)
	}
	writeUndoResponse(w, r, http.StatusOK, result)
}

func completeActionUndo(store ActionUndoStore, operationID, state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return store.CompleteUndo(ctx, operationID, state)
}

func writeUndoResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response[any]{Data: data, RequestID: requestID})
}
