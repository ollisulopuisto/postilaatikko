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
