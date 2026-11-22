package message

import "testing"

func TestDeduplicateThreadMessagesPreservesScopeAndAccountBoundaries(t *testing.T) {
	context := &Message{ID: "context", AccountID: "a", MessageID: "<same@example.org>", BodyFetched: true}
	matched := &Message{ID: "matched", AccountID: "a", MessageID: "same@example.org"}
	other := &Message{ID: "other", AccountID: "b", MessageID: "<same@example.org>"}
	missing := &Message{ID: "missing", AccountID: "a"}
	missing2 := &Message{ID: "missing2", AccountID: "a"}
	result := DeduplicateThreadMessages([]*Message{context, other, matched, missing, missing2}, "", map[string]bool{"matched": true})
	if len(result) != 4 || result[0] != matched || result[1] != other || result[2] != missing || result[3] != missing2 {
		t.Fatalf("scope, account or missing-RFC-ID identity lost: %+v", result)
	}
	if context.BodyFetched != true || matched.BodyFetched != false {
		t.Fatal("selection mutated source copies")
	}
}

func TestDeduplicateThreadMessagesPrefersLiveOverCachedSnapshot(t *testing.T) {
	var snapshotUID int32 = -7
	snapshot := &Message{ID: "snapshot", AccountID: "a", FolderID: "inbox", MessageID: "same@test", UID: uint32(snapshotUID), BodyFetched: true, BodyText: "downloaded body"}
	live := &Message{ID: "live", AccountID: "a", FolderID: "inbox", MessageID: "same@test", UID: 7}
	for _, copies := range [][]*Message{{snapshot, live}, {live, snapshot}} {
		got := DeduplicateThreadMessages(copies, "inbox", nil)

		if len(got) != 1 || got[0].ID != live.ID || got[0].UID != live.UID || !got[0].BodyFetched || got[0].BodyText != snapshot.BodyText || got[0].CachedBodySourceID != snapshot.ID {
			t.Fatal("cached snapshot hid live reader target", got)
		}
	}
}

func TestCachedReaderOverlayNeverMutatesSourceRows(t *testing.T) {
	var negative int32 = -1
	cached := &Message{ID: "snapshot", AccountID: "a", MessageID: "same@test", UID: uint32(negative), BodyText: "offline", BodyFetched: true}
	live := &Message{ID: "live", AccountID: "a", MessageID: "same@test", UID: 8}
	_ = DeduplicateThreadMessages([]*Message{cached, live}, "", nil)
	if live.BodyFetched || live.BodyText != "" || cached.UID != uint32(negative) {
		t.Fatal("presentation changed source identities or cache state")
	}
}

func TestReaderDoesNotAttributeEncryptedSnapshotToLiveHeaders(t *testing.T) {
	ms, db := setupThreadTestDB(t)
	defer db.Close()
	for _, msg := range []*Message{{ID: "cached", AccountID: "acc-1", FolderID: "fld-1", UID: 7, MessageID: "encrypted@test", BodyFetched: true}, {ID: "live", AccountID: "acc-1", FolderID: "fld-1", UID: 8, MessageID: "encrypted@test"}} {
		if err := ms.Create(msg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE messages SET uid=-1,smime_raw_body=?,smime_encrypted=1 WHERE id='cached'`, []byte("encrypted fixture")); err != nil {
		t.Fatal(err)
	}
	got, err := ms.GetReaderMessage("live")
	if err != nil || got.CachedBodySourceID != "" || got.HasSMIME || got.SMIMEEncrypted || got.BodyFetched {
		t.Fatal("live headers inherited unproven snapshot verification", got, err)
	}
}

func TestSignedSnapshotCannotAuthenticateSpoofedLiveHeaders(t *testing.T) {
	var negative int32 = -1
	for _, mode := range []string{"smime", "pgp"} {
		cached := &Message{ID: "cached", AccountID: "a", FolderID: "inbox", UID: uint32(negative), MessageID: "reused@test", BodyFetched: true, BodyText: "signed content", FromEmail: "real@test"}
		if mode == "smime" {
			cached.HasSMIME = true
			cached.SMIMEStatus = "valid"
		} else {
			cached.HasPGP = true
			cached.PGPStatus = "valid"
		}
		live := &Message{ID: "live", AccountID: "a", FolderID: "inbox", UID: 8, MessageID: "reused@test", FromEmail: "spoof@test"}
		got := DeduplicateThreadMessages([]*Message{cached, live}, "inbox", nil)[0]
		if got.BodyFetched || got.CachedBodySourceID != "" || got.HasPGP || got.HasSMIME || got.SMIMEStatus != "" || got.PGPStatus != "" {
			t.Fatal("cached verification attributed to unrelated headers", mode, got)
		}
	}
}

func TestReaderDoesNotOverlayRowsWithPartialCryptoMetadata(t *testing.T) {
	mutations := map[string]func(*Message){
		"smime-encrypted": func(m *Message) { m.SMIMEEncrypted = true },
		"pgp-encrypted":   func(m *Message) { m.PGPEncrypted = true },
		"smime-signer":    func(m *Message) { m.SMIMESignerEmail = "signer@test" },
		"pgp-signer":      func(m *Message) { m.PGPSignerKeyID = "fixture" },
	}
	for name, mutate := range mutations {
		for _, target := range []string{"cached", "live"} {
			t.Run(name+"/"+target, func(t *testing.T) {
				var negative int32 = -1
				cached := &Message{ID: "cached", AccountID: "a", UID: uint32(negative), MessageID: "same@test", BodyFetched: true, BodyText: "offline"}
				live := &Message{ID: "live", AccountID: "a", UID: 8, MessageID: "same@test"}
				if target == "cached" {
					mutate(cached)
				} else {
					mutate(live)
				}
				got := DeduplicateThreadMessages([]*Message{cached, live}, "", nil)[0]
				if got.BodyFetched || got.CachedBodySourceID != "" {
					t.Fatal("partial crypto metadata overlaid", got)
				}
			})
		}
	}
}
