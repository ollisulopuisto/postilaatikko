package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/operations"
)

// failingMarkReadOperations wraps the standard mock with a MarkAsRead failure.
type failingMarkReadOperations struct {
	*mockActionOperations
}

func (m *failingMarkReadOperations) MarkAsRead(messageIDs []string) error {
	return errors.New("provider rejected the flag update")
}

func archiveReadRequest(ids ...string) v1.ActionRequest {
	refs := make([]operations.MessageRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, operations.MessageRef{AccountID: "acc-1", MessageID: id})
	}
	return v1.ActionRequest{
		Action: v1.ActionArchiveMarkRead,
		Target: operations.Target{Kind: operations.TargetMatchedMessages, Messages: refs},
	}
}

func postAction(t *testing.T, mux *http.ServeMux, body v1.ActionRequest, key string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/actions", bytes.NewReader(payload))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestActionsHandler_ArchiveMarkRead(t *testing.T) {
	ops := &mockActionOperations{
		archiveOperationID: "op-arch-read-001",
		messages: map[string]*message.Message{
			"msg-1": {ID: "msg-1", AccountID: "acc-1"},
			"msg-2": {ID: "msg-2", AccountID: "acc-1"},
		},
	}
	mux, mock := setupActionsTestServer(ops, map[v1.Capability]bool{v1.CapabilityMailWrite: true})

	rec := postAction(t, mux, archiveReadRequest("msg-1", "msg-2"), "idem-archive-read-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp v1.Response[v1.ActionResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Data.Receipt.OperationID != "op-arch-read-001" {
		t.Errorf("expected archive operation ID op-arch-read-001, got %s", resp.Data.Receipt.OperationID)
	}
	if len(mock.archived) != 2 || mock.archived[0] != "msg-1" || mock.archived[1] != "msg-2" {
		t.Errorf("archive_read must archive the requested messages, archived %v", mock.archived)
	}
	if len(mock.markedRead) != 2 || mock.markedRead[0] != "msg-1" || mock.markedRead[1] != "msg-2" {
		t.Errorf("archive_read must mark the requested messages read, marked %v", mock.markedRead)
	}
}

func TestActionsHandler_ArchiveMarkRead_MarkReadFailureSurfaces(t *testing.T) {
	base := &mockActionOperations{
		archiveOperationID: "op-arch-read-002",
		messages: map[string]*message.Message{
			"msg-1": {ID: "msg-1", AccountID: "acc-1"},
		},
	}
	ops := &failingMarkReadOperations{mockActionOperations: base}
	handler := v1.NewActionsHandler(ops)
	directMux := http.NewServeMux()
	handler.MountRoutes(directMux, func(w http.ResponseWriter, r *http.Request, cap v1.Capability) bool { return true })
	rec := postAction(t, directMux, archiveReadRequest("msg-1"), "idem-archive-read-2")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when mark-read fails after archive, got %d", rec.Code)
	}
	if len(base.archived) != 1 {
		t.Errorf("archive leg must have run before the failing mark-read, archived %v", base.archived)
	}
}

// groupedArchiveReadOperations is a grouped durable-path fake: one provider
// group, a captured Undo snapshot, and executed group bookkeeping.
type groupedArchiveReadOperations struct {
	*mockActionOperations
	mu           sync.Mutex
	executed     []string
	snapshotReqs []v1.ActionRequest
}

func (m *groupedArchiveReadOperations) PlanActionGroups(ctx context.Context, req v1.ActionRequest, ids []string) ([]v1.ActionProviderGroup, error) {
	return []v1.ActionProviderGroup{{
		ID:                   "group-1",
		AccountID:            "acc-1",
		SourceFolderID:       "folder-inbox",
		DestinationAccountID: "acc-1",
		DestinationFolderID:  "folder-archive",
		MessageIDs:           append([]string(nil), ids...),
	}}, nil
}

func (m *groupedArchiveReadOperations) ExecuteActionGroup(ctx context.Context, req v1.ActionRequest, group v1.ActionProviderGroup) error {
	if req.Action != v1.ActionArchiveMarkRead {
		return errors.New("unexpected action")
	}
	if _, err := m.ArchiveMessages(group.MessageIDs); err != nil {
		return err
	}
	if err := m.MarkAsRead(group.MessageIDs); err != nil {
		return err
	}
	m.mu.Lock()
	m.executed = append(m.executed, group.ID)
	m.mu.Unlock()
	return nil
}

func (m *groupedArchiveReadOperations) CaptureActionUndo(ctx context.Context, req v1.ActionRequest, ids []string) ([]byte, error) {
	m.mu.Lock()
	m.snapshotReqs = append(m.snapshotReqs, req)
	m.mu.Unlock()
	payload, err := json.Marshal(map[string]any{
		"action": req.Action,
		"entries": []map[string]any{{
			"accountId":         "acc-1",
			"messageId":         ids[0],
			"folderId":          "folder-inbox",
			"expectedAccountId": "acc-1",
			"expectedFolderId":  "folder-archive",
			"isRead":            false,
			"expectedIsRead":    true,
		}},
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (m *groupedArchiveReadOperations) CombineActionUndoSnapshots(ctx context.Context, snapshots [][]byte) ([]byte, error) {
	if len(snapshots) != 1 {
		return nil, errors.New("expected exactly one confirmed snapshot")
	}
	return snapshots[0], nil
}

// fakeGroupReceipts is an in-memory ActionProviderGroupReceiptStore.
type fakeGroupReceipts struct {
	mu            sync.Mutex
	reserved      bool
	reserveAction v1.ActionKind
	groups        []v1.ActionProviderGroup
	groupStates   []string
	completed     bool
	undoPayload   []byte
}

func (s *fakeGroupReceipts) Lookup(ctx context.Context, keyHash string) (v1.ActionReceiptRecord, bool, error) {
	return v1.ActionReceiptRecord{}, false, nil
}

func (s *fakeGroupReceipts) Reserve(ctx context.Context, keyHash, requestHash, operationID string, action v1.ActionKind, count int) (v1.ActionReceiptRecord, bool, error) {
	return v1.ActionReceiptRecord{}, false, errors.New("grouped actions must reserve groups")
}

func (s *fakeGroupReceipts) ReserveActionGroups(ctx context.Context, keyHash, requestHash, operationID string, action v1.ActionKind, count int, affected []v1.AffectedMessageRef, groups []v1.ActionProviderGroup) (v1.ActionReceiptRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved = true
	s.reserveAction = action
	s.groups = append([]v1.ActionProviderGroup(nil), groups...)
	return v1.ActionReceiptRecord{OperationID: operationID, RequestHash: requestHash}, true, nil
}

func (s *fakeGroupReceipts) SetActionGroupState(ctx context.Context, operationID, groupID, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groupStates = append(s.groupStates, state)
	return nil
}

func (s *fakeGroupReceipts) Complete(ctx context.Context, keyHash, requestHash string, action v1.ActionKind, state v1.OperationState, status int, payload []byte, undoSnapshot []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed = true
	s.undoPayload = append([]byte(nil), undoSnapshot...)
	return nil
}

func TestActionsHandler_ArchiveMarkRead_GroupedDurableUndo(t *testing.T) {
	ops := &groupedArchiveReadOperations{
		mockActionOperations: &mockActionOperations{
			messages: map[string]*message.Message{
				"msg-1": {ID: "msg-1", AccountID: "acc-1", FolderID: "folder-inbox"},
			},
		},
	}
	receipts := &fakeGroupReceipts{}
	handler := v1.NewActionsHandlerWithReceiptStore(ops, receipts, func(key string) string { return "digest:" + key })
	mux := http.NewServeMux()
	handler.MountRoutes(mux, func(w http.ResponseWriter, r *http.Request, cap v1.Capability) bool { return true })

	rec := postAction(t, mux, archiveReadRequest("msg-1"), "idem-archive-read-3")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp v1.Response[v1.ActionResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !resp.Data.UndoAvailable {
		t.Errorf("archive_read must expose Undo for the confirmed group")
	}
	if resp.Data.UndoExpiresAt == "" {
		t.Errorf("archive_read Undo must carry an expiry")
	}
	if len(resp.Data.Groups) != 1 || resp.Data.Groups[0].State != v1.ActionGroupConfirmed {
		t.Errorf("expected one confirmed group, got %+v", resp.Data.Groups)
	}

	if len(ops.executed) != 1 || ops.executed[0] != "group-1" {
		t.Errorf("expected the provider group to execute once, got %v", ops.executed)
	}
	if len(ops.markedRead) != 1 || ops.markedRead[0] != "msg-1" {
		t.Errorf("grouped archive_read must mark the group read, marked %v", ops.markedRead)
	}
	if len(ops.snapshotReqs) != 1 || ops.snapshotReqs[0].Action != v1.ActionArchiveMarkRead {
		t.Errorf("Undo snapshot must be captured for archive_read, got %+v", ops.snapshotReqs)
	}

	receipts.mu.Lock()
	defer receipts.mu.Unlock()
	if !receipts.reserved || receipts.reserveAction != v1.ActionArchiveMarkRead {
		t.Errorf("receipt must be reserved for archive_read")
	}
	if !receipts.completed {
		t.Errorf("receipt must be completed")
	}
	var snapshot map[string]any
	if err := json.Unmarshal(receipts.undoPayload, &snapshot); err != nil {
		t.Fatalf("stored Undo payload is unreadable: %v", err)
	}
	if snapshot["action"] != string(v1.ActionArchiveMarkRead) {
		t.Errorf("stored Undo payload must belong to archive_read, got %v", snapshot["action"])
	}
}
