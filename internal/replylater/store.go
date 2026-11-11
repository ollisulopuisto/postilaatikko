// Package replylater stores explicit conversation membership in the personal
// Reply Later triage queue. Membership is independent of provider replied flags.
package replylater

import (
	"context"
	"fmt"
	"strings"

	"github.com/hkdb/aerion/internal/database"
)

// Ref identifies a conversation in one account. Conversation IDs may be
// provider thread IDs or local message IDs for single-message conversations.
type Ref struct {
	AccountID      string `json:"accountId"`
	ConversationID string `json:"conversationId"`
	// Indexed reports whether a message row for this conversation currently
	// exists in the local message index. It is populated by List and lets the
	// UI distinguish recoverable stale queue entries from unloaded pages.
	Indexed bool `json:"indexed"`
}

// Store persists queue membership across app restarts.
type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func (s *Store) Add(ref Ref) (bool, error) {
	if err := validate(ref); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(context.Background(), `
		INSERT INTO reply_later_queue(account_id, conversation_id)
		VALUES (?, ?) ON CONFLICT(account_id, conversation_id) DO NOTHING`,
		ref.AccountID, ref.ConversationID)
	if err != nil {
		return false, fmt.Errorf("add Reply Later conversation: %w", err)
	}
	changed, err := result.RowsAffected()
	return changed > 0, err
}

func (s *Store) Remove(ref Ref) (bool, error) {
	if err := validate(ref); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(context.Background(), `
		DELETE FROM reply_later_queue WHERE account_id = ? AND conversation_id = ?`,
		ref.AccountID, ref.ConversationID)
	if err != nil {
		return false, fmt.Errorf("remove Reply Later conversation: %w", err)
	}
	changed, err := result.RowsAffected()
	return changed > 0, err
}

func (s *Store) Contains(ref Ref) (bool, error) {
	if err := validate(ref); err != nil {
		return false, err
	}
	var exists int
	err := s.db.QueryRowContext(context.Background(), `
		SELECT EXISTS(SELECT 1 FROM reply_later_queue WHERE account_id = ? AND conversation_id = ?)`,
		ref.AccountID, ref.ConversationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check Reply Later membership: %w", err)
	}
	return exists == 1, nil
}

// List returns all queue references, including stale ones whose local message
// rows are currently missing. The UI can report those instead of silently
// dropping a user's triage item.
func (s *Store) List() ([]Ref, error) {
	rows, err := s.db.QueryContext(context.Background(), `
		SELECT rl.account_id, rl.conversation_id,
			EXISTS (
				SELECT 1 FROM messages m
				WHERE m.account_id = rl.account_id
				  AND (COALESCE(m.thread_id, m.id) = rl.conversation_id OR m.id = rl.conversation_id)
			) AS indexed
		FROM reply_later_queue rl
		ORDER BY rl.added_at DESC, rl.account_id, rl.conversation_id`)
	if err != nil {
		return nil, fmt.Errorf("list Reply Later conversations: %w", err)
	}
	defer rows.Close()
	refs := make([]Ref, 0)
	for rows.Next() {
		var ref Ref
		if err := rows.Scan(&ref.AccountID, &ref.ConversationID, &ref.Indexed); err != nil {
			return nil, fmt.Errorf("scan Reply Later conversation: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Reply Later conversations: %w", err)
	}
	return refs, nil
}

func (s *Store) Count() (int, error) {
	var count int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM reply_later_queue`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count Reply Later conversations: %w", err)
	}
	return count, nil
}

func validate(ref Ref) error {
	if strings.TrimSpace(ref.AccountID) == "" || strings.TrimSpace(ref.ConversationID) == "" {
		return fmt.Errorf("account and conversation IDs are required")
	}
	return nil
}
