package operations

import (
	"errors"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/message"
)

// mockLookup implements MessageLookup for testing.
type mockLookup struct {
	messages map[string]*message.Message   // local ID -> message
	threads  map[string]map[string]bool    // accountID:threadID -> set of local IDs
	failGet  bool
}

func (m *mockLookup) GetByIDs(ids []string) ([]*message.Message, error) {
	if m.failGet {
		return nil, errors.New("store read failure")
	}
	var out []*message.Message
	for _, id := range ids {
		if msg, ok := m.messages[id]; ok {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (m *mockLookup) GetMessageIDsInThread(accountID, threadID string) ([]string, error) {
	key := accountID + ":" + threadID
	ids := make([]string, 0, len(m.threads[key]))
	for id := range m.threads[key] {
		ids = append(ids, id)
	}
	return ids, nil
}

func newMessage(localID, accountID, folderID, threadID string) *message.Message {
	return &message.Message{
		ID:        localID,
		AccountID: accountID,
		FolderID:  folderID,
		ThreadID:  threadID,
	}
}

func TestResolveMessageTargetSingleValid(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
		},
	}
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}},
	}
	ids, err := Resolve(target, lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("Resolve() = %v, want [m1]", ids)
	}
}

func TestResolveMessageTargetMultipleRejected(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
			"m2": newMessage("m2", "acct-a", "f1", "t1"),
		},
	}
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}, {AccountID: "acct-a", MessageID: "m2"}},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "exactly one message") {
		t.Fatalf("Resolve() error = %v, want 'exactly one message'", err)
	}
}

func TestResolveMessageTargetEmptyRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{Kind: TargetMessage, Messages: nil}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "at least one message") {
		t.Fatalf("Resolve() error = %v, want 'at least one message'", err)
	}
}

func TestResolveMessageTargetWithThreadRefRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}},
		Thread:   &ThreadRef{AccountID: "acct-a", ThreadID: "t1"},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "thread reference") {
		t.Fatalf("Resolve() error = %v, want 'thread reference'", err)
	}
}

func TestResolveMatchedMessagesTargetValid(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
			"m2": newMessage("m2", "acct-a", "f1", "t1"),
			"m3": newMessage("m3", "acct-b", "f2", "t2"),
		},
	}
	target := Target{
		Kind:     TargetMatchedMessages,
		Messages: []MessageRef{
			{AccountID: "acct-a", MessageID: "m1"},
			{AccountID: "acct-b", MessageID: "m3"},
		},
	}
	ids, err := Resolve(target, lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("Resolve() = %v, want 2 ids", ids)
	}
}

func TestResolveThreadTargetValid(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
			"m2": newMessage("m2", "acct-a", "f1", "t1"),
			"m3": newMessage("m3", "acct-a", "f2", "t2"),
		},
		threads: map[string]map[string]bool{
			"acct-a:t1": {"m1": true, "m2": true},
		},
	}
	target := Target{
		Kind:   TargetThread,
		Thread: &ThreadRef{AccountID: "acct-a", ThreadID: "t1"},
	}
	ids, err := Resolve(target, lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("Resolve() = %v, want 2 thread message ids", ids)
	}
}

func TestResolveThreadTargetWithMessagesRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{
		Kind:     TargetThread,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}},
		Thread:   &ThreadRef{AccountID: "acct-a", ThreadID: "t1"},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "message references") {
		t.Fatalf("Resolve() error = %v, want 'message references'", err)
	}
}

func TestResolveThreadTargetMissingAccountOrThreadIDRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{Kind: TargetThread, Thread: &ThreadRef{AccountID: "", ThreadID: "t1"}}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "thread target needs accountId and threadId") {
		t.Fatalf("Resolve() error = %v, want 'thread target needs accountId and threadId'", err)
	}
}

func TestResolveMessageTargetAccountMismatchRejected(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
		},
	}
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-b", MessageID: "m1"}},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "does not belong to account") {
		t.Fatalf("Resolve() error = %v, want 'does not belong to account'", err)
	}
}

func TestResolveMessageTargetMissingFieldsRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{Kind: TargetMessage, Messages: []MessageRef{{AccountID: "", MessageID: "m1"}}}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "accountId and messageId") {
		t.Fatalf("Resolve() error = %v, want 'accountId and messageId'", err)
	}
}

func TestResolveMessageTargetDuplicateRejected(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{
			"m1": newMessage("m1", "acct-a", "f1", "t1"),
		},
	}
	target := Target{
		Kind:     TargetMatchedMessages,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}, {AccountID: "acct-a", MessageID: "m1"}},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "duplicate message reference") {
		t.Fatalf("Resolve() error = %v, want 'duplicate message reference'", err)
	}
}

func TestResolveMessageTargetNotFoundRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "no longer exist") {
		t.Fatalf("Resolve() error = %v, want 'no longer exist'", err)
	}
}

func TestResolveUnknownKindRejected(t *testing.T) {
	lookup := &mockLookup{messages: map[string]*message.Message{}}
	target := Target{Kind: "bogus", Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}}}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "unknown mail target kind") {
		t.Fatalf("Resolve() error = %v, want 'unknown mail target kind'", err)
	}
}

func TestResolveNilLookupRejected(t *testing.T) {
	target := Target{
		Kind:     TargetMessage,
		Messages: []MessageRef{{AccountID: "acct-a", MessageID: "m1"}},
	}
	_, err := Resolve(target, nil)
	if err == nil || !strings.Contains(err.Error(), "message lookup is unavailable") {
		t.Fatalf("Resolve() error = %v, want 'message lookup is unavailable'", err)
	}
}

func TestResolveThreadNotFoundRejected(t *testing.T) {
	lookup := &mockLookup{
		messages: map[string]*message.Message{},
		threads:  map[string]map[string]bool{},
	}
	target := Target{
		Kind:   TargetThread,
		Thread: &ThreadRef{AccountID: "acct-a", ThreadID: "t99"},
	}
	_, err := Resolve(target, lookup)
	if err == nil || !strings.Contains(err.Error(), "target thread no longer exists") {
		t.Fatalf("Resolve() error = %v, want 'target thread no longer exists'", err)
	}
}
