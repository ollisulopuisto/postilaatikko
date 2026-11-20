package alwayson

import (
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"testing"
)

type bootstrapKeysetProbe struct {
	calls int
	v1.ConversationListOperations
}

func (p *bootstrapKeysetProbe) ListConversationsUnifiedInboxKeyset(string, int) ([]*message.Conversation, string, error) {
	p.calls++
	return []*message.Conversation{}, "", nil
}
func (p *bootstrapKeysetProbe) ListConversationsByFolderKeyset(string, string, int) ([]*message.Conversation, string, error) {
	panic("unexpected folder")
}
func (p *bootstrapKeysetProbe) ListConversationsUnifiedFolderKeyset(string, string, int) ([]*message.Conversation, string, error) {
	panic("unexpected unified folder")
}
func TestWorkspaceBootstrapPreservesKeysetAdapter(t *testing.T) {
	probe := &bootstrapKeysetProbe{}
	ops := workspaceBootstrapOperations{ConversationListOperations: probe, ConversationListKeysetOperations: probe}
	// Catalog dependencies are not needed for the type-assertion route check.
	_ = ops
	if _, ok := any(ops).(v1.ConversationListKeysetOperations); !ok {
		t.Fatal("bootstrap erased keyset capability")
	}
	if _, _, err := ops.ListConversationsUnifiedInboxKeyset("", 50); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatal("bootstrap did not forward keyset call")
	}
}
