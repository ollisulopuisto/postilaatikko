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
		  current_node_id TEXT NOT NULL REFERENCES draft_history_nodes(node_id));`)
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
