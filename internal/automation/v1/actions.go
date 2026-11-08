package v1

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hkdb/aerion/internal/operations"
)

// ErrActionReceiptConflict means an idempotency key already belongs to a
// different canonical action request.
var ErrActionReceiptConflict = errors.New("action idempotency key conflict")

// ActionKind represents a valid mail action type.
type ActionKind string

const (
	ActionArchive     ActionKind = "archive"
	ActionMarkRead    ActionKind = "mark_read"
	ActionMarkUnread  ActionKind = "mark_unread"
	ActionStar        ActionKind = "star"
	ActionUnstar      ActionKind = "unstar"
	ActionTrash       ActionKind = "trash"
	ActionMove        ActionKind = "move"
	ActionMoveToInbox ActionKind = "move_to_inbox"
	ActionMoveToSpam  ActionKind = "move_to_spam"
)

// ActionRequest specifies a mutation on an explicit target.
type ActionRequest struct {
	Action      ActionKind        `json:"action"`
	Target      operations.Target `json:"target"`
	Destination string            `json:"destination,omitempty"` // Required for "move"
}

// ActionResponse describes the outcome and provides an operation receipt.
type ActionResponse struct {
	Receipt       OperationReceipt     `json:"receipt"`
	MessageIDs    []string             `json:"messageIds"`
	Affected      []AffectedMessageRef `json:"affected,omitempty"`
	UndoAvailable bool                 `json:"undoAvailable,omitempty"`
	UndoExpiresAt string               `json:"undoExpiresAt,omitempty"`
}

// AffectedMessageRef identifies a changed mailbox row without exposing content.
type AffectedMessageRef struct {
	AccountID string `json:"accountId"`
	MessageID string `json:"messageId"`
	FolderID  string `json:"folderId"`
}

// ActionAffectedRefReader reads bounded-safe references after an action.
type ActionAffectedRefReader interface {
	ActionAffectedRefs(context.Context, []string) ([]AffectedMessageRef, error)
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

// ActionInboxOperations supports account-specific Inbox resolution for a
// cross-account move-to-Inbox action.
type ActionInboxOperations interface {
	MoveMessagesToInbox(messageIDs []string) error
}

// ActionSpamOperations supports account-specific Spam-folder resolution.
type ActionSpamOperations interface {
	MoveMessagesToSpam(messageIDs []string) error
}

// ActionsHandler executes mail mutations with idempotency checking and target validation.
type ActionsHandler struct {
	ops             ActionOperations
	receipts        ActionReceiptStore
	keyDigest       func(string) string
	mu              sync.Mutex
	idempotencyKeys map[IdempotencyKey]*actionIdempotencyRecord
}

// ActionReceiptRecord is the durable replay state for one helper-owned action.
type ActionReceiptRecord struct {
	OperationID string
	RequestHash string
	State       OperationState
	Status      int
	Response    []byte
}

// ActionReceiptStore reserves an operation before provider effects and stores
// its terminal or uncertain receipt for safe retries.
type ActionReceiptStore interface {
	Lookup(context.Context, string) (ActionReceiptRecord, bool, error)
	Reserve(context.Context, string, string, string, ActionKind, int) (ActionReceiptRecord, bool, error)
	Complete(context.Context, string, string, ActionKind, OperationState, int, []byte, []byte) error
}

// ActionUndoSnapshotter captures the helper-owned prior state needed to reverse
// a mutation. The serialized result must contain identifiers/flags only.
type ActionUndoSnapshotter interface {
	CaptureActionUndo(context.Context, ActionRequest, []string) ([]byte, error)
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

// NewActionsHandlerWithReceiptStore initializes a handler with durable action
// idempotency. keyDigest must return a one-way digest of the caller key.
func NewActionsHandlerWithReceiptStore(ops ActionOperations, store ActionReceiptStore, keyDigest func(string) string) *ActionsHandler {
	h := NewActionsHandler(ops)
	h.receipts = store
	h.keyDigest = keyDigest
	return h
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
	if h.receipts != nil && h.keyDigest != nil {
		h.handleDurableAction(w, r, idempotencyKey, hex.EncodeToString(requestHash[:]), req)
		return
	}

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

func (h *ActionsHandler) handleDurableAction(w http.ResponseWriter, r *http.Request, key IdempotencyKey, requestHash string, req ActionRequest) {
	keyHash := h.keyDigest(string(key))
	stored, exists, err := h.receipts.Lookup(r.Context(), keyHash)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read action receipt", true)
		return
	}
	if exists {
		h.writeStoredAction(w, r, stored, requestHash)
		return
	}
	if req.Action == ActionMove && strings.TrimSpace(req.Destination) == "" {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Destination folder ID required for move action", false)
		return
	}
	switch req.Action {
	case ActionArchive, ActionMarkRead, ActionMarkUnread, ActionStar, ActionUnstar, ActionTrash, ActionMove, ActionMoveToInbox, ActionMoveToSpam:
	default:
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Unknown action %q", req.Action), false)
		return
	}
	messageIDs, err := operations.Resolve(req.Target, h.ops)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Target resolution failed: %v", err), false)
		return
	}
	operationID := generateOpID()
	stored, owner, err := h.receipts.Reserve(r.Context(), keyHash, requestHash, operationID, req.Action, len(messageIDs))
	if err != nil {
		if errors.Is(err, ErrActionReceiptConflict) {
			h.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different action request", false)
			return
		}
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not reserve action receipt", true)
		return
	}
	if !owner {
		h.writeStoredAction(w, r, stored, requestHash)
		return
	}

	var undoSnapshot []byte
	if snapshotter, ok := h.ops.(ActionUndoSnapshotter); ok {
		undoSnapshot, err = snapshotter.CaptureActionUndo(r.Context(), req, messageIDs)
		if err != nil {
			state := OperationUnknown
			response := ActionResponse{Receipt: OperationReceipt{OperationID: operationID, State: state}, MessageIDs: messageIDs}
			payload, _ := json.Marshal(response)
			completeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if completeErr := h.receipts.Complete(completeCtx, keyHash, requestHash, req.Action, state, http.StatusOK, payload, nil); completeErr != nil {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not record action receipt", true)
				return
			}
			h.writeResponse(w, r, http.StatusOK, response)
			return
		}
	}
	opErr := h.performAction(req, messageIDs, &operationID)
	state := OperationSent
	if opErr != nil {
		// Provider errors may be ambiguous. Record uncertainty and never run the
		// operation again for this key without explicit reconciliation.
		state = OperationUnknown
	}
	response := ActionResponse{
		Receipt:       OperationReceipt{OperationID: operationID, State: state},
		MessageIDs:    messageIDs,
		UndoAvailable: state == OperationSent && len(undoSnapshot) > 0,
	}
	if state == OperationSent {
		if reader, ok := h.ops.(ActionAffectedRefReader); ok {
			response.Affected, _ = reader.ActionAffectedRefs(r.Context(), messageIDs)
		}
	}
	if response.UndoAvailable {
		response.UndoExpiresAt = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not encode action receipt", false)
		return
	}
	completeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.receipts.Complete(completeCtx, keyHash, requestHash, req.Action, state, http.StatusOK, payload, undoSnapshot); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Action completed but its receipt could not be saved", true)
		return
	}
	h.writeResponse(w, r, http.StatusOK, response)
}

func (h *ActionsHandler) writeStoredAction(w http.ResponseWriter, r *http.Request, stored ActionReceiptRecord, requestHash string) {
	if stored.RequestHash != requestHash {
		h.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different action request", false)
		return
	}
	if len(stored.Response) > 0 {
		var response ActionResponse
		if err := json.Unmarshal(stored.Response, &response); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Stored action receipt is unreadable", false)
			return
		}
		response.Receipt.Replayed = true
		h.writeResponse(w, r, stored.Status, response)
		return
	}
	state := OperationUnknown
	if stored.State == "processing" {
		state = OperationSending
	}
	response := ActionResponse{Receipt: OperationReceipt{OperationID: stored.OperationID, State: state, Replayed: true}}
	h.writeResponse(w, r, http.StatusOK, response)
}

func (h *ActionsHandler) performAction(req ActionRequest, messageIDs []string, operationID *string) error {
	switch req.Action {
	case ActionArchive:
		id, err := h.ops.ArchiveMessages(messageIDs)
		if id != "" {
			*operationID = id
		}
		return err
	case ActionMarkRead:
		return h.ops.MarkAsRead(messageIDs)
	case ActionMarkUnread:
		return h.ops.MarkAsUnread(messageIDs)
	case ActionStar:
		return h.ops.StarMessages(messageIDs)
	case ActionUnstar:
		return h.ops.UnstarMessages(messageIDs)
	case ActionTrash:
		return h.ops.TrashMessages(messageIDs)
	case ActionMove:
		id, err := h.ops.MoveMessages(messageIDs, req.Destination)
		if id != "" {
			*operationID = id
		}
		return err
	case ActionMoveToInbox:
		inboxOperations, ok := h.ops.(ActionInboxOperations)
		if !ok {
			return errors.New("move-to-Inbox action is unsupported")
		}
		return inboxOperations.MoveMessagesToInbox(messageIDs)
	case ActionMoveToSpam:
		spamOperations, ok := h.ops.(ActionSpamOperations)
		if !ok {
			return errors.New("move-to-Spam action is unsupported")
		}
		return spamOperations.MoveMessagesToSpam(messageIDs)
	default:
		return fmt.Errorf("unsupported action %q", req.Action)
	}
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
