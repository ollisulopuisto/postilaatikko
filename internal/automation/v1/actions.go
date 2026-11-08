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
	ActionArchive          ActionKind = "archive"
	ActionMarkRead         ActionKind = "mark_read"
	ActionMarkUnread       ActionKind = "mark_unread"
	ActionStar             ActionKind = "star"
	ActionUnstar           ActionKind = "unstar"
	ActionTrash            ActionKind = "trash"
	ActionMove             ActionKind = "move"
	ActionMoveToInbox      ActionKind = "move_to_inbox"
	ActionMoveToSpam       ActionKind = "move_to_spam"
	ActionReplyLaterAdd    ActionKind = "reply_later_add"
	ActionReplyLaterRemove ActionKind = "reply_later_remove"
	ActionReadLaterAdd     ActionKind = "read_later_add"
	ActionReadLaterRemove  ActionKind = "read_later_remove"
	ActionSnooze           ActionKind = "snooze"
	ActionUnsnooze         ActionKind = "unsnooze"
)

// ActionRequest specifies a mutation on an explicit target.
type ActionRequest struct {
	Action      ActionKind        `json:"action"`
	Target      operations.Target `json:"target"`
	Destination string            `json:"destination,omitempty"` // Required for "move"
}

// ActionResponse describes the outcome and provides an operation receipt.
type ActionResponse struct {
	Receipt        OperationReceipt     `json:"receipt"`
	MessageIDs     []string             `json:"messageIds"`
	Affected       []AffectedMessageRef `json:"affected,omitempty"`
	AffectedTriage []AffectedTriageRef  `json:"affectedTriage,omitempty"`
	Partial        bool                 `json:"partial,omitempty"`
	Groups         []ActionGroupOutcome `json:"groups,omitempty"`
	UndoAvailable  bool                 `json:"undoAvailable,omitempty"`
	UndoExpiresAt  string               `json:"undoExpiresAt,omitempty"`
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
	Affected    []AffectedMessageRef
	Groups      []ActionGroupOutcome
}

// ActionReceiptStore reserves an operation before provider effects and stores
// its terminal or uncertain receipt for safe retries.
type ActionReceiptStore interface {
	Lookup(context.Context, string) (ActionReceiptRecord, bool, error)
	Reserve(context.Context, string, string, string, ActionKind, int) (ActionReceiptRecord, bool, error)
	Complete(context.Context, string, string, ActionKind, OperationState, int, []byte, []byte) error
}

// ActionTargetReceiptStore atomically stores content-free target references
// alongside a durable operation reservation before provider effects begin.
type ActionTargetReceiptStore interface {
	ReserveAction(context.Context, string, string, string, ActionKind, int, []AffectedMessageRef) (ActionReceiptRecord, bool, error)
}

// ActionProviderGroup is one independently checkpointed provider mutation.
// Message IDs and the Undo snapshot remain helper-owned and are never returned
// in ActionGroupOutcome.
type ActionProviderGroup struct {
	ID                   string
	AccountID            string
	SourceFolderID       string
	DestinationAccountID string
	DestinationFolderID  string
	MessageIDs           []string
	UndoPayload          []byte
	State                string
}

// ActionGroupOutcome is the content-free public state of one provider group.
type ActionGroupOutcome struct {
	ID                   string `json:"id"`
	AccountID            string `json:"accountId"`
	SourceFolderID       string `json:"sourceFolderId"`
	DestinationAccountID string `json:"destinationAccountId,omitempty"`
	DestinationFolderID  string `json:"destinationFolderId,omitempty"`
	State                string `json:"state"`
	MessageCount         int    `json:"messageCount"`
}

const (
	ActionGroupPlanned        = "planned"
	ActionGroupProcessing     = "processing"
	ActionGroupConfirmed      = "confirmed"
	ActionGroupOutcomeUnknown = "outcome_unknown"
	ActionGroupNotStarted     = "not_started"
)

// ActionProviderGroupOperations plans and runs a single account/folder provider group.
type ActionProviderGroupOperations interface {
	PlanActionGroups(context.Context, ActionRequest, []string) ([]ActionProviderGroup, error)
	ExecuteActionGroup(context.Context, ActionRequest, ActionProviderGroup) error
}

// ActionProviderGroupReceiptStore reserves group intents and checkpoints each provider result.
type ActionProviderGroupReceiptStore interface {
	ReserveActionGroups(context.Context, string, string, string, ActionKind, int, []AffectedMessageRef, []ActionProviderGroup) (ActionReceiptRecord, bool, error)
	SetActionGroupState(context.Context, string, string, string) error
}

// ActionGroupUndoSnapshotCombiner joins confirmed per-group snapshots into the
// payload accepted by the helper-owned Undo executor.
type ActionGroupUndoSnapshotCombiner interface {
	CombineActionUndoSnapshots(context.Context, [][]byte) ([]byte, error)
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
	var affectedBefore []AffectedMessageRef
	reader, hasAffectedReader := h.ops.(ActionAffectedRefReader)
	_, hasTargetStore := h.receipts.(ActionTargetReceiptStore)
	if hasTargetStore && !hasAffectedReader {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Action targets cannot be recorded for recovery", false)
		return
	}
	if hasAffectedReader {
		affectedBefore, err = reader.ActionAffectedRefs(r.Context(), messageIDs)
		if err != nil {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read action targets for recovery", true)
			return
		}
		if len(affectedBefore) != len(messageIDs) {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not resolve all action targets for recovery", false)
			return
		}
		for _, ref := range affectedBefore {
			if ref.AccountID == "" || ref.MessageID == "" || ref.FolderID == "" {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Action target reference is incomplete", false)
				return
			}
		}
	}
	operationID := generateOpID()
	if groupOperations, ok := h.ops.(ActionProviderGroupOperations); ok {
		groupReceipts, hasGroupReceipts := h.receipts.(ActionProviderGroupReceiptStore)
		if !hasGroupReceipts {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider group receipts are unavailable", false)
			return
		}
		h.handleGroupedDurableAction(w, r, keyHash, requestHash, operationID, req, messageIDs, affectedBefore, groupOperations, groupReceipts)
		return
	}
	var owner bool
	if targetStore, ok := h.receipts.(ActionTargetReceiptStore); ok {
		stored, owner, err = targetStore.ReserveAction(r.Context(), keyHash, requestHash, operationID, req.Action, len(messageIDs), affectedBefore)
	} else {
		stored, owner, err = h.receipts.Reserve(r.Context(), keyHash, requestHash, operationID, req.Action, len(messageIDs))
	}
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
			response := ActionResponse{Receipt: OperationReceipt{OperationID: operationID, State: state}, MessageIDs: messageIDs, Affected: affectedBefore}
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
		if hasAffectedReader {
			if affectedAfter, readErr := reader.ActionAffectedRefs(r.Context(), messageIDs); readErr == nil {
				response.Affected = affectedAfter
			} else {
				response.Affected = affectedBefore
			}
		}
	} else {
		response.Affected = affectedBefore
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

func (h *ActionsHandler) handleGroupedDurableAction(w http.ResponseWriter, r *http.Request, keyHash, requestHash, operationID string, req ActionRequest, messageIDs []string, affectedBefore []AffectedMessageRef, operations ActionProviderGroupOperations, receipts ActionProviderGroupReceiptStore) {
	groups, err := operations.PlanActionGroups(r.Context(), req, messageIDs)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Could not plan provider action groups: %v", err), false)
		return
	}
	if len(groups) == 0 {
		h.writeError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Action has no provider groups", false)
		return
	}
	requested := make(map[string]struct{}, len(messageIDs))
	for _, messageID := range messageIDs {
		requested[messageID] = struct{}{}
	}
	if len(requested) != len(messageIDs) {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Resolved action targets contain duplicates", false)
		return
	}
	seen := make(map[string]struct{}, len(messageIDs))
	groupRefs := make([]AffectedMessageRef, 0, len(messageIDs))
	for _, group := range groups {
		if group.ID == "" || group.AccountID == "" || group.SourceFolderID == "" || len(group.MessageIDs) == 0 {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider action group has incomplete identity", false)
			return
		}
		for _, messageID := range group.MessageIDs {
			if _, exists := requested[messageID]; !exists {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider action group contains an unexpected target", false)
				return
			}
			if _, exists := seen[messageID]; exists {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider action groups contain a duplicate target", false)
				return
			}
			seen[messageID] = struct{}{}
			groupRefs = append(groupRefs, AffectedMessageRef{AccountID: group.AccountID, MessageID: messageID, FolderID: group.SourceFolderID})
		}
	}
	if len(seen) != len(requested) {
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider action groups do not cover every target", false)
		return
	}
	affectedBefore = uniqueAffectedMessageRefs(groupRefs)
	snapshotter, hasSnapshotter := h.ops.(ActionUndoSnapshotter)
	for i := range groups {
		if hasSnapshotter {
			groups[i].UndoPayload, err = snapshotter.CaptureActionUndo(r.Context(), req, groups[i].MessageIDs)
			if err != nil {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not capture provider group Undo state", true)
				return
			}
		}
	}
	stored, owner, err := receipts.ReserveActionGroups(r.Context(), keyHash, requestHash, operationID, req.Action, len(messageIDs), affectedBefore, groups)
	if err != nil {
		if errors.Is(err, ErrActionReceiptConflict) {
			h.writeError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different action request", false)
			return
		}
		h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not reserve provider action groups", true)
		return
	}
	if !owner {
		h.writeStoredAction(w, r, stored, requestHash)
		return
	}

	confirmed := 0
	unknown := 0
	for i := range groups {
		if err := receipts.SetActionGroupState(r.Context(), operationID, groups[i].ID, ActionGroupProcessing); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not checkpoint provider action start", true)
			return
		}
		groups[i].State = ActionGroupProcessing
		if err := operations.ExecuteActionGroup(r.Context(), req, groups[i]); err != nil {
			groups[i].State = ActionGroupOutcomeUnknown
			unknown++
			if checkpointErr := receipts.SetActionGroupState(r.Context(), operationID, groups[i].ID, ActionGroupOutcomeUnknown); checkpointErr != nil {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider result is uncertain and could not be checkpointed", true)
				return
			}
			continue
		}
		groups[i].State = ActionGroupConfirmed
		confirmed++
		if err := receipts.SetActionGroupState(r.Context(), operationID, groups[i].ID, ActionGroupConfirmed); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Provider success could not be checkpointed", true)
			return
		}
	}

	state := OperationSent
	if unknown > 0 {
		state = OperationUnknown
	}
	response := ActionResponse{
		Receipt:       OperationReceipt{OperationID: operationID, State: state},
		MessageIDs:    messageIDs,
		Affected:      affectedBefore,
		Partial:       confirmed > 0 && unknown > 0,
		UndoAvailable: confirmed > 0 && hasSnapshotter,
	}
	response.Groups = make([]ActionGroupOutcome, 0, len(groups))
	moveAction := req.Action == ActionArchive || req.Action == ActionTrash || req.Action == ActionMove || req.Action == ActionMoveToInbox || req.Action == ActionMoveToSpam
	if moveAction {
		response.Affected = append([]AffectedMessageRef(nil), affectedBefore...)
	}
	confirmedSnapshots := make([][]byte, 0, confirmed)
	for _, group := range groups {
		response.Groups = append(response.Groups, ActionGroupOutcome{
			ID:                   group.ID,
			AccountID:            group.AccountID,
			SourceFolderID:       group.SourceFolderID,
			DestinationAccountID: group.DestinationAccountID,
			DestinationFolderID:  group.DestinationFolderID,
			State:                group.State,
			MessageCount:         len(group.MessageIDs),
		})
		if group.State == ActionGroupConfirmed && len(group.UndoPayload) > 0 {
			confirmedSnapshots = append(confirmedSnapshots, group.UndoPayload)
		}
	}
	if reader, ok := h.ops.(ActionAffectedRefReader); ok {
		if affectedAfter, readErr := reader.ActionAffectedRefs(r.Context(), messageIDs); readErr == nil && len(affectedAfter) == len(messageIDs) {
			response.Affected = append(response.Affected, affectedAfter...)
		}
	}
	if moveAction {
		for _, group := range groups {
			if group.DestinationAccountID == "" || group.DestinationFolderID == "" {
				continue
			}
			for _, messageID := range group.MessageIDs {
				response.Affected = append(response.Affected, AffectedMessageRef{AccountID: group.DestinationAccountID, MessageID: messageID, FolderID: group.DestinationFolderID})
			}
		}
	}
	response.Affected = uniqueAffectedMessageRefs(response.Affected)
	var undoSnapshot []byte
	if response.UndoAvailable {
		if combiner, ok := h.ops.(ActionGroupUndoSnapshotCombiner); ok {
			undoSnapshot, err = combiner.CombineActionUndoSnapshots(r.Context(), confirmedSnapshots)
			if err != nil {
				h.writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not combine confirmed Undo state", true)
				return
			}
		} else if len(confirmedSnapshots) == 1 {
			undoSnapshot = confirmedSnapshots[0]
		} else {
			response.UndoAvailable = false
		}
		if len(undoSnapshot) == 0 {
			response.UndoAvailable = false
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

func uniqueAffectedMessageRefs(refs []AffectedMessageRef) []AffectedMessageRef {
	seen := make(map[AffectedMessageRef]struct{}, len(refs))
	unique := make([]AffectedMessageRef, 0, len(refs))
	for _, ref := range refs {
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		unique = append(unique, ref)
	}
	return unique
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
	response := ActionResponse{Receipt: OperationReceipt{OperationID: stored.OperationID, State: state, Replayed: true}, Affected: stored.Affected, Groups: stored.Groups}
	response.Partial = hasConfirmedActionGroup(stored.Groups) && hasNonconfirmedActionGroup(stored.Groups)
	h.writeResponse(w, r, http.StatusOK, response)
}

func hasConfirmedActionGroup(groups []ActionGroupOutcome) bool {
	for _, group := range groups {
		if group.State == ActionGroupConfirmed {
			return true
		}
	}
	return false
}

func hasNonconfirmedActionGroup(groups []ActionGroupOutcome) bool {
	for _, group := range groups {
		if group.State != ActionGroupConfirmed {
			return true
		}
	}
	return false
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
