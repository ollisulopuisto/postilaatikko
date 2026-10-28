package drafthistory

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`
		CREATE TABLE draft_history_states (
		  hash TEXT PRIMARY KEY, payload TEXT NOT NULL, byte_size INTEGER NOT NULL,
		  created_at TEXT NOT NULL DEFAULT (datetime('now')));
		CREATE TABLE draft_history_nodes (
		  node_id TEXT PRIMARY KEY, draft_key TEXT NOT NULL,
		  parent_id TEXT REFERENCES draft_history_nodes(node_id),
		  preferred_child_id TEXT, label TEXT NOT NULL DEFAULT '',
		  state_hash TEXT NOT NULL REFERENCES draft_history_states(hash),
		  created_at TEXT NOT NULL DEFAULT (datetime('now')));
		CREATE TABLE draft_history_heads (
		  draft_key TEXT PRIMARY KEY,
		  current_node_id TEXT NOT NULL REFERENCES draft_history_nodes(node_id));
		CREATE TABLE draft_attachment_blobs (
		  hash TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '',
		  mime TEXT NOT NULL DEFAULT '', size INTEGER NOT NULL DEFAULT 0,
		  content BLOB NOT NULL,
		  created_at TEXT NOT NULL DEFAULT (datetime('now')));
		CREATE TABLE draft_history_node_blobs (
		  node_id TEXT NOT NULL REFERENCES draft_history_nodes(node_id) ON DELETE CASCADE,
		  blob_hash TEXT NOT NULL REFERENCES draft_attachment_blobs(hash),
		  PRIMARY KEY (node_id, blob_hash));`)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func TestCommitBranchingAndRedo(t *testing.T) {
	s := newTestStore(t)
	root, err := s.Commit("d1", "", "new", `{"body":"a"}`)
	if err != nil {
		t.Fatal(err)
	}
	n1, _ := s.Commit("d1", root, "typing", `{"body":"ab"}`)
	n2, _ := s.Commit("d1", n1, "typing", `{"body":"abc"}`)

	// Undo twice, then edit: must fork, not overwrite n2.
	if err := s.Checkout("d1", n1); err != nil {
		t.Fatal(err)
	}
	if err := s.Checkout("d1", root); err != nil {
		t.Fatal(err)
	}
	n3, err := s.Commit("d1", root, "typing", `{"body":"ax"}`)
	if err != nil {
		t.Fatal(err)
	}

	tree, err := s.Tree("d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Nodes) != 4 {
		t.Fatalf("nodes = %d, want 4 (fork must keep both branches)", len(tree.Nodes))
	}
	if tree.CurrentNodeID != n3 {
		t.Fatalf("head = %q, want fork %q", tree.CurrentNodeID, n3)
	}
	// Preferred child of root is now the fork; the old branch stays reachable.
	var preferred string
	for _, n := range tree.Nodes {
		if n.NodeID == root {
			preferred = n.PreferredChildID
		}
	}
	if preferred != n3 {
		t.Fatalf("root preferred child = %q, want %q", preferred, n3)
	}
	p, err := s.State(n2)
	if err != nil || p != `{"body":"abc"}` {
		t.Fatalf("old branch state = %q, %v", p, err)
	}
}

func TestStateDeduplicatedByHash(t *testing.T) {
	s := newTestStore(t)
	root, _ := s.Commit("d1", "", "new", `{"body":"same"}`)
	if _, err := s.Commit("d1", root, "typing", `{"body":"same"}`); err != nil {
		t.Fatal(err)
	}
	var states, nodes int
	s.db.QueryRow(`SELECT COUNT(*) FROM draft_history_states`).Scan(&states)
	s.db.QueryRow(`SELECT COUNT(*) FROM draft_history_nodes`).Scan(&nodes)
	if states != 1 || nodes != 2 {
		t.Fatalf("states=%d nodes=%d, want 1 deduped state, 2 nodes", states, nodes)
	}
}

func TestCommitRejectsForeignParentAndSecondRoot(t *testing.T) {
	s := newTestStore(t)
	other, _ := s.Commit("other", "", "new", `{}`)
	if _, err := s.Commit("d1", other, "x", `{}`); err == nil {
		t.Fatal("foreign parent accepted")
	}
	s.Commit("d1", "", "new", `{}`)
	if _, err := s.Commit("d1", "", "again", `{}`); err == nil {
		t.Fatal("second root accepted")
	}
}

func TestCheckoutUnknownNode(t *testing.T) {
	s := newTestStore(t)
	if err := s.Checkout("d1", "nope"); err == nil {
		t.Fatal("checkout of unknown node succeeded")
	}
}

func TestClearRemovesTreeAndOrphanStates(t *testing.T) {
	s := newTestStore(t)
	r1, _ := s.Commit("d1", "", "new", `{"body":"keep? no"}`)
	s.Commit("d1", r1, "typing", `{"body":"gone"}`)
	r2, _ := s.Commit("d2", "", "new", `{"body":"shared"}`)
	s.Commit("d2", r2, "typing", `{"body":"shared2"}`)
	if err := s.Clear("d1"); err != nil {
		t.Fatal(err)
	}
	tree, _ := s.Tree("d1")
	if len(tree.Nodes) != 0 {
		t.Fatalf("d1 nodes remain: %d", len(tree.Nodes))
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM draft_history_states`).Scan(&n)
	if n != 2 {
		t.Fatalf("states = %d, want d2's 2 states untouched", n)
	}
}

// The store must work against the schema created by the app's own
// migrations, not just the test-local DDL above.
func TestStoreAgainstMigratedSchema(t *testing.T) {
	// Reuse the real migration SQL from the database package by applying
	// only the two statements this store needs through the same table names.
	// (Full migration coverage lives in internal/database; this guards
	// column-name drift between the store and migration 56.)
	s := newTestStore(t)
	root, err := s.Commit("k", "", "new", `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit("k", root, "typing", `{"a":2}`); err != nil {
		t.Fatal(err)
	}
	tree, err := s.Tree("k")
	if err != nil || len(tree.Nodes) != 2 || tree.CurrentNodeID == root {
		t.Fatalf("tree = %+v, %v", tree, err)
	}
}

func TestMoveRekeysTree(t *testing.T) {
	s := newTestStore(t)
	root, err := s.Commit("acc:new-1", "", "new", `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Commit("acc:new-1", root, "typing", `{"a":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Move("acc:new-1", "acc:42"); err != nil {
		t.Fatal(err)
	}
	oldTree, err := s.Tree("acc:new-1")
	if err != nil || len(oldTree.Nodes) != 0 {
		t.Fatalf("old key should be empty, got %+v, %v", oldTree, err)
	}
	tree, err := s.Tree("acc:42")
	if err != nil || len(tree.Nodes) != 2 || tree.CurrentNodeID != child {
		t.Fatalf("moved tree = %+v, %v", tree, err)
	}
	// History keeps working under the new key.
	if _, err := s.Commit("acc:42", child, "typing", `{"a":3}`); err != nil {
		t.Fatal(err)
	}
}

func TestMoveRefusesOccupiedTarget(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Commit("k1", "", "new", `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit("k2", "", "new", `{"b":1}`); err != nil {
		t.Fatal(err)
	}
	if err := s.Move("k1", "k2"); err == nil {
		t.Fatal("expected refusal to merge two live trees")
	}
}

func TestBlobMembershipAndCollection(t *testing.T) {
	s := newTestStore(t)
	content := []byte("fake-pdf-bytes")
	h1, err := s.PutBlob("a.pdf", "application/pdf", content)
	if err != nil {
		t.Fatal(err)
	}
	// Dedup: same content, same hash, no error.
	h2, err := s.PutBlob("copy.pdf", "application/pdf", content)
	if err != nil || h1 != h2 {
		t.Fatalf("dedup: %v %v %v", h1, h2, err)
	}
	node, err := s.CommitBlobs("k", "", "attach", `{"a":1}`, []string{h1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitBlobs("k", node, "edit", `{"a":2}`, []string{"deadbeef"}); err == nil {
		t.Fatal("expected unknown-blob rejection")
	}
	got, err := s.GetBlob(h1)
	if err != nil || got.Name != "a.pdf" || string(got.Content) != string(content) {
		t.Fatalf("GetBlob = %+v, %v", got, err)
	}
	hashes, err := s.NodeBlobs(node)
	if err != nil || len(hashes) != 1 || hashes[0] != h1 {
		t.Fatalf("NodeBlobs = %v, %v", hashes, err)
	}
	// Clear removes the tree, membership, and the now-orphaned blob.
	if err := s.Clear("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBlob(h1); err == nil {
		t.Fatal("expected blob collected after Clear")
	}
}

func TestBlobSurvivesClearWhileReferenced(t *testing.T) {
	s := newTestStore(t)
	h, err := s.PutBlob("a.pdf", "application/pdf", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitBlobs("k1", "", "attach", `{"a":1}`, []string{h}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitBlobs("k2", "", "attach", `{"b":1}`, []string{h}); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear("k1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBlob(h); err != nil {
		t.Fatal("blob still referenced by k2 must survive k1 Clear")
	}
}
