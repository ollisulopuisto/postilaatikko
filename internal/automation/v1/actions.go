package v1

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hkdb/aerion/internal/operations"
)

// ActionKind represents a valid mail action type.
type ActionKind string

const (
	ActionArchive    ActionKind = "archive"
	ActionMarkRead   ActionKind = "mark_read"
	ActionMarkUnread ActionKind = "mark_unread"
	ActionStar       ActionKind = "star"
	ActionUnstar     ActionKind = "unstar"
	ActionTrash      ActionKind = "trash"
	ActionMove       ActionKind = "move"
)

// ActionRequest specifies a mutation on an explicit target.
type ActionRequest struct {
	Action      ActionKind        `json:"action"`
	Target      operations.Target `json:"target"`
	Destination string            `json:"destination,omitempty"` // Required for "move"
}

// ActionResponse describes the outcome and provides an operation receipt.
type ActionResponse struct {
	Receipt    OperationReceipt `json:"receipt"`
	MessageIDs []string         `json:"messageIds"`
}

// ActionOperations defines the mutations supported by the backend mail service.
type ActionOperations interface {
	operations.MessageLookup
	ArchiveMessages(messageIDs []string) (string, error)
	MarkAsRead(messageIDs []string) error
	MarkAsUnread(messageIDs []string) error
	StarMessages(messageIDs []string) error
	UnstarMessages(messageIDs []string) error
	TrashMessages(messageIDs []string) error
	MoveMessages(messageIDs []string, destinationFolderID string) (string, error)
}

// ActionsHandler executes mail mutations with idempotency checking and target validation.
type ActionsHandler struct {
	ops             ActionOperations
	mu              sync.Mutex
	idempotencyKeys map[IdempotencyKey]*actionIdempotencyRecord
}

type actionIdempotencyRecord struct {
	requestHash [32]byte
	done        chan struct{}
	status      int
	response    *ActionResponse
	errorCode   ErrorCode
	errorText   string
	retryable   bool
}

// NewActionsHandler initializes an ActionsHandler.
func NewActionsHandler(ops ActionOperations) *ActionsHandler {
	return &ActionsHandler{
		ops:             ops,
		idempotencyKeys: make(map[IdempotencyKey]*actionIdempotencyRecord),
	}
}

// MountRoutes mounts the action endpoints onto a standard ServeMux.
func (h *ActionsHandler) MountRoutes(mux *http.ServeMux, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/actions", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			h.writeError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		h.handleAction(w, r)
	})
}

func (h *ActionsHandler) handleAction(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := IdempotencyKey(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if idempotencyKey == "" {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Idempotency-Key header is required for mutations", false)
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Malformed request: %v", err), false)
		return
	}
	encodedRequest, err := json.Marshal(req)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed action request", false)
		return
	}
	requestHash := sha256.Sum256(encodedRequest)

	h.mu.Lock()
	if existing := h.idempotencyKeys[idempotencyKey]; existing != nil {
		if existing.requestHash != requestHash {
			h.mu.Unlock()
			h.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different action request", false)
			return
		}
		done := existing.done
		h.mu.Unlock()
		select {
		case <-done:
			h.writeIdempotentResult(w, r, existing, true)
		case <-r.Context().Done():
			h.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Matching action is still in progress", true)
		}
		return
	}
	record := &actionIdempotencyRecord{requestHash: requestHash, done: make(chan struct{})}
	h.idempotencyKeys[idempotencyKey] = record
	h.mu.Unlock()
	finished := false
	defer func() {
		if !finished {
			h.finishError(record, http.StatusInternalServerError, CodeInternal, "Action stopped before its result was recorded", true)
		}
	}()

	messageIDs, err := operations.Resolve(req.Target, h.ops)
	if err != nil {
		h.finishError(record, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Target resolution failed: %v", err), false)
		finished = true
		h.writeIdempotentResult(w, r, record, false)
		return
	}

	operationID := generateOpID()
	var opErr error

	switch req.Action {
	case ActionArchive:
		id, err := h.ops.ArchiveMessages(messageIDs)
		if id != "" {
			operationID = id
		}
		opErr = err

	case ActionMarkRead:
		opErr = h.ops.MarkAsRead(messageIDs)

	case ActionMarkUnread:
		opErr = h.ops.MarkAsUnread(messageIDs)

	case ActionStar:
		opErr = h.ops.StarMessages(messageIDs)

	case ActionUnstar:
		opErr = h.ops.UnstarMessages(messageIDs)

	case ActionTrash:
		opErr = h.ops.TrashMessages(messageIDs)

	case ActionMove:
		if strings.TrimSpace(req.Destination) == "" {
			h.finishError(record, http.StatusBadRequest, CodeInvalidRequest, "Destination folder ID required for move action", false)
			finished = true
			h.writeIdempotentResult(w, r, record, false)
			return
		}
		id, err := h.ops.MoveMessages(messageIDs, req.Destination)
		if id != "" {
			operationID = id
		}
		opErr = err

	default:
		h.finishError(record, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Unknown action %q", req.Action), false)
		finished = true
		h.writeIdempotentResult(w, r, record, false)
		return
	}

	if opErr != nil {
		h.finishError(record, http.StatusInternalServerError, CodeInternal, fmt.Sprintf("Action failed: %v", opErr), true)
		finished = true
		h.writeIdempotentResult(w, r, record, false)
		return
	}

	resp := ActionResponse{
		Receipt: OperationReceipt{
			OperationID: operationID,
			State:       OperationSent,
			Replayed:    false,
		},
		MessageIDs: messageIDs,
	}

	h.mu.Lock()
	record.status = http.StatusOK
	record.response = &resp
	close(record.done)
	h.mu.Unlock()
	finished = true
	h.writeIdempotentResult(w, r, record, false)
}

func (h *ActionsHandler) finishError(record *actionIdempotencyRecord, status int, code ErrorCode, message string, retryable bool) {
	h.mu.Lock()
	record.status = status
	record.errorCode = code
	record.errorText = message
	record.retryable = retryable
	close(record.done)
	h.mu.Unlock()
}

func (h *ActionsHandler) writeIdempotentResult(w http.ResponseWriter, r *http.Request, record *actionIdempotencyRecord, replay bool) {
	if record.response != nil {
		response := *record.response
		response.Receipt.Replayed = replay
		h.writeResponse(w, r, record.status, response)
		return
	}
	h.writeError(w, r, record.status, record.errorCode, record.errorText, record.retryable)
}

func (h *ActionsHandler) writeResponse(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	resp := Response[any]{
		Data:      data,
		RequestID: reqID,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ActionsHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code ErrorCode, message string, retryable bool) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	resp := ErrorResponse{
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			Retryable: retryable,
			RequestID: reqID,
		},
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

func generateOpID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("op-%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}
