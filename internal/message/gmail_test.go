package message

import (
	"testing"
	"time"
)

func TestGmailHeadersPersistWithoutIMAPAndKeepBodies(t *testing.T) {
	s, a, f := newBodyFailedTestStore(t)
	m := &Message{AccountID: a, FolderID: f, MessageID: "<remote>", ThreadID: "gmail:thread", Subject: "Remote", FromEmail: "peer@example.org", Date: time.Now(), IsRead: false}
	if err := s.UpsertGmail(m, "remote-id"); err != nil {
		t.Fatal(err)
	}
	loadedHeader, err := s.Get(m.ID)
	if err != nil || loadedHeader.BodyFetched {
		t.Fatalf("header-only Gmail row already fetched: %+v %v", loadedHeader, err)
	}
	if m.UID == 0 || m.UID != loadedHeader.UID {
		t.Fatalf("callback UID %d stored UID %d", m.UID, loadedHeader.UID)
	}
	if m.ID == "" {
		t.Fatal("no local id")
	}
	if err := s.UpdateBodiesBatch([]BodyUpdate{{MessageID: m.ID, BodyText: "Opened content"}}); err != nil {
		t.Fatal(err)
	}
	next := &Message{AccountID: a, FolderID: f, MessageID: "<remote>", ThreadID: "gmail:thread", Subject: "Remote", Date: m.Date, IsRead: true}
	if err := s.UpsertGmail(next, "remote-id"); err != nil {
		t.Fatal(err)
	}
	if next.UID != m.UID {
		t.Fatalf("refresh UID %d changed from %d", next.UID, m.UID)
	}
	if next.ID != m.ID {
		t.Fatal("identity changed")
	}
	loaded, err := s.Get(m.ID)
	if err != nil || loaded.BodyText != "Opened content" || !loaded.IsRead {
		t.Fatalf("lost content/flags: %+v %v", loaded, err)
	}
	id, err := s.GmailID(a, m.ID)
	if err != nil || id != "remote-id" {
		t.Fatalf("mapping %q %v", id, err)
	}
	id, err = s.GmailID("other-account", m.ID)
	if err != nil || id != "" {
		t.Fatalf("cross-account leak %q %v", id, err)
	}
}

func TestHeaderRefreshReturnsCanonicalIDForListID(t *testing.T) {
	s, a, f := newBodyFailedTestStore(t)
	original := &Message{ID: "canonical", AccountID: a, FolderID: f, UID: 12, Date: time.Now()}
	if err := s.Upsert(original); err != nil {
		t.Fatal(err)
	}
	refreshed := &Message{AccountID: a, FolderID: f, UID: 12, Date: time.Now(), Subject: "Refreshed"}
	if err := s.Upsert(refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.ID != original.ID {
		t.Fatalf("refresh returned transient ID %q", refreshed.ID)
	}
	if err := s.SetListID(refreshed.ID, "news.example.com"); err != nil {
		t.Fatal(err)
	}
	value, err := s.GetListID(original.ID)
	if err != nil || value != "news.example.com" {
		t.Fatalf("backfill %q %v", value, err)
	}
}
