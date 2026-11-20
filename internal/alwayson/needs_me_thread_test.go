package alwayson

import (
	"context"
	"testing"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
)

func TestGetThreadWithTriageScope(t *testing.T) {
	ops, db := setupThreadReadTest(t)
	defer db.Close()
	ops.runtime.accounts = account.NewStore(db)
	ops.runtime.folders = folder.NewStore(db)
	scope := views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: "triage"}
	view, err := ops.GetThread(context.Background(), views.ThreadRef{AccountID: "acc-1", ThreadID: "thread-current"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if view.MatchedCount != 1 || len(view.Messages) != 2 || !view.Messages[0].Matched || view.Messages[1].Matched {
		t.Fatalf("triage scope should match inbox, not sent context: %#v", view)
	}
	// Membership follows the same mappings, hidden state and enabled state as the list.
	for _, update := range []string{
		`UPDATE folders SET hidden=1 WHERE id='inbox'`,
		`UPDATE folders SET hidden=0,folder_type='archive' WHERE id='inbox'`,
		`UPDATE folders SET folder_type='inbox'; UPDATE accounts SET enabled=0`,
	} {
		if _, err = db.Exec(update); err != nil {
			t.Fatal(err)
		}
		msg, getErr := ops.runtime.messages.Get("msg-inbox")
		if getErr != nil {
			t.Fatal(getErr)
		}
		matched, scopeErr := ops.messageInScope(msg, scope)
		if scopeErr != nil || matched {
			t.Fatalf("excluded message matched: %v %v", matched, scopeErr)
		}
	}
}
