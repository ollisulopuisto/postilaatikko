package drafthistory

// Kuulas-style branching undo history for composer drafts.
//
// Every accepted edit is a node holding the whole composer state (body,
// subject, recipients). States are stored once, keyed by the SHA-256 of
// their canonical JSON, so undo/redo cycles and identical states cost no
// extra rows - but node/pointer rows still accrue per edit, and no
// automatic pruning is performed. Undo never destroys: editing after undo
// forks a new branch off the current node, and every branch stays
// reachable through Tree(). Redo follows the parent's preferred child
// (the branch most recently descended into).
//
// Lifetime policy (persist across restarts vs session-only) is settled per
// draft: the store keeps everything until Clear is called; wiring Clear to
// send/discard lifecycle is a separate, pending product decision.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store persists draft undo trees in the app database.
type Store struct {
	db *sql.DB
}

// NewStore wraps an existing database handle.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Node is one committed composer state.
type Node struct {
	NodeID           string `json:"nodeId"`
	DraftKey         string `json:"draftKey"`
	ParentID         string `json:"parentId"` // empty for the root
	PreferredChildID string `json:"preferredChildId"`
	Label            string `json:"label"` // short op description: typing, paste, format, recipients, subject
	StateHash        string `json:"stateHash"`
	CreatedAt        string `json:"createdAt"`
}

// Tree is the full branch structure plus the current pointer.
type Tree struct {
	Nodes         []*Node `json:"nodes"`
	CurrentNodeID string  `json:"currentNodeId"`
}

func stateHash(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// DraftHistoryCommit atomically stores the state (deduplicated by hash),
// appends a node under parentID, marks it the parent's preferred child and
// moves the head. parentID must be the caller's current node: passing the
// head is what makes edit-after-undo fork a new branch. An empty parentID
// starts a new tree. Returns the new node ID.
func (s *Store) Commit(draftKey, parentID, label, payload string) (string, error) {
	if draftKey == "" {
		return "", fmt.Errorf("draft history: empty draft key")
	}
	if len(label) > 200 {
		label = label[:200]
	}
	hash := stateHash(payload)
	nodeID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return "", fmt.Errorf("draft history commit: %w", err)
	}
	defer tx.Rollback()

	if parentID == "" {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM draft_history_nodes WHERE draft_key = ?`, draftKey).Scan(&n); err != nil {
			return "", err
		}
		if n > 0 {
			return "", fmt.Errorf("draft history: tree for %q already has a root", draftKey)
		}
	} else {
		var owner string
		err := tx.QueryRow(`SELECT draft_key FROM draft_history_nodes WHERE node_id = ?`, parentID).Scan(&owner)
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("draft history: unknown parent node %q", parentID)
		}
		if err != nil {
			return "", err
		}
		if owner != draftKey {
			return "", fmt.Errorf("draft history: parent node belongs to another draft")
		}
	}

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO draft_history_states (hash, payload, byte_size, created_at) VALUES (?,?,?,?)`,
		hash, payload, len(payload), now); err != nil {
		return "", err
	}
	var parent any
	if parentID != "" {
		parent = parentID
	}
	if _, err := tx.Exec(
		`INSERT INTO draft_history_nodes (node_id, draft_key, parent_id, label, state_hash, created_at) VALUES (?,?,?,?,?,?)`,
		nodeID, draftKey, parent, label, hash, now); err != nil {
		return "", err
	}
	if parentID != "" {
		if _, err := tx.Exec(
			`UPDATE draft_history_nodes SET preferred_child_id = ? WHERE node_id = ?`, nodeID, parentID); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO draft_history_heads (draft_key, current_node_id) VALUES (?,?)
		 ON CONFLICT(draft_key) DO UPDATE SET current_node_id = excluded.current_node_id`,
		draftKey, nodeID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("draft history commit: %w", err)
	}
	return nodeID, nil
}

// Tree returns every node of a draft's tree and its head.
func (s *Store) Tree(draftKey string) (*Tree, error) {
	rows, err := s.db.Query(
		`SELECT node_id, draft_key, COALESCE(parent_id,''), COALESCE(preferred_child_id,''), label, state_hash, created_at
		 FROM draft_history_nodes WHERE draft_key = ? ORDER BY created_at, node_id`, draftKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tree := &Tree{Nodes: []*Node{}}
	for rows.Next() {
		n := &Node{}
		if err := rows.Scan(&n.NodeID, &n.DraftKey, &n.ParentID, &n.PreferredChildID, &n.Label, &n.StateHash, &n.CreatedAt); err != nil {
			return nil, err
		}
		tree.Nodes = append(tree.Nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = s.db.QueryRow(`SELECT current_node_id FROM draft_history_heads WHERE draft_key = ?`, draftKey).Scan(&tree.CurrentNodeID)
	return tree, nil
}

// DraftHistoryState returns the stored composer state payload for a node.
func (s *Store) State(nodeID string) (string, error) {
	var payload string
	err := s.db.QueryRow(
		`SELECT s.payload FROM draft_history_states s
		 JOIN draft_history_nodes n ON n.state_hash = s.hash WHERE n.node_id = ?`, nodeID).Scan(&payload)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("draft history: unknown node %q", nodeID)
	}
	return payload, err
}

// DraftHistoryCheckout moves the head to an existing node (undo/redo/goto).
// Moving to a child records it as the parent's preferred child, so redo
// follows the last-descended branch. No state is written.
func (s *Store) Checkout(draftKey, nodeID string) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parent string
	err = tx.QueryRow(`SELECT COALESCE(parent_id,'') FROM draft_history_nodes WHERE node_id = ? AND draft_key = ?`, nodeID, draftKey).Scan(&parent)
	if err == sql.ErrNoRows {
		return fmt.Errorf("draft history: unknown node %q for draft", nodeID)
	}
	if err != nil {
		return err
	}
	if parent != "" {
		if _, err := tx.Exec(`UPDATE draft_history_nodes SET preferred_child_id = ? WHERE node_id = ?`, nodeID, parent); err != nil {
			return err
		}
	}
	res, err := tx.Exec(
		`INSERT INTO draft_history_heads (draft_key, current_node_id) VALUES (?,?)
		 ON CONFLICT(draft_key) DO UPDATE SET current_node_id = excluded.current_node_id`,
		draftKey, nodeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("draft history: failed to move head")
	}
	return tx.Commit()
}

// ClearDraftHistory deletes a draft's whole tree. Lifetime policy is
// undecided; nothing calls this yet.
func (s *Store) Clear(draftKey string) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM draft_history_nodes WHERE draft_key = ?`, draftKey); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM draft_history_heads WHERE draft_key = ?`, draftKey); err != nil {
		return err
	}
	// States are shared by hash across nodes of potentially other drafts;
	// remove only newly orphaned ones.
	if _, err := tx.Exec(
		`DELETE FROM draft_history_states WHERE hash NOT IN (SELECT DISTINCT state_hash FROM draft_history_nodes)`); err != nil {
		return err
	}
	return tx.Commit()
}

// Move re-keys an entire tree from oldKey to newKey. Used when an unsaved
// "new" draft first gets a real draft ID: the pre-save states committed
// under the temporary key stay reachable instead of being orphaned. It is
// a no-op when no tree exists under oldKey. If a tree already exists under
// newKey (e.g. a restored draft was edited before the re-key arrived), the
// caller must resolve that conflict before calling; Move refuses rather
// than merging two live trees.
func (s *Store) Move(oldKey, newKey string) error {
	if oldKey == newKey {
		return nil
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM draft_history_nodes WHERE draft_key = ?`, newKey).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return fmt.Errorf("draft history: target key %q already has a tree", newKey)
	}
	if _, err := tx.Exec(`UPDATE draft_history_nodes SET draft_key = ? WHERE draft_key = ?`, newKey, oldKey); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE draft_history_heads SET draft_key = ? WHERE draft_key = ?`, newKey, oldKey); err != nil {
		return err
	}
	return tx.Commit()
}
