package app

import (
	"context"
	"testing"

	"github.com/hkdb/aerion/internal/virtualfolder"
)

func TestGetVirtualFolderUnreadCountsUsesConnectedHelper(t *testing.T) {
	called := false
	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true}
		},
		unreadCountsFunc: func(context.Context) (map[string]int, error) {
			called = true
			return map[string]int{"view-server": 4}, nil
		},
	}
	app := &App{mailboxService: connector}
	counts, err := app.GetVirtualFolderUnreadCounts()
	if err != nil {
		t.Fatalf("GetVirtualFolderUnreadCounts: %v", err)
	}
	if !called || counts["view-server"] != 4 || len(counts) != 1 {
		t.Fatalf("called=%v counts=%#v, want helper counts", called, counts)
	}
}

func TestGetVirtualFolderUnreadCountsFailsClosedForRemoteFailure(t *testing.T) {
	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{ErrorCode: "remote_unavailable", Message: "remote helper unavailable"}
		},
	}
	app := &App{mailboxService: connector}
	if _, err := app.GetVirtualFolderUnreadCounts(); err == nil {
		t.Fatal("GetVirtualFolderUnreadCounts succeeded with a remote failure")
	}
}

func TestGetVirtualFolderUnreadCountsFallsBackWhenLocalHelperUnavailable(t *testing.T) {
	db := openMailboxMutationTestDB(t)
	app := &App{
		mailboxService: &mailboxServiceConnector{statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{ErrorCode: "service_unavailable"}
		}},
		virtualFolderStore: virtualfolder.NewStore(db),
	}
	counts, err := app.GetVirtualFolderUnreadCounts()
	if err != nil {
		t.Fatalf("GetVirtualFolderUnreadCounts local fallback: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("local fallback counts = %#v, want an empty map", counts)
	}
}
