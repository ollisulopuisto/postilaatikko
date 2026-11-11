package message

import (
	"context"
	"fmt"
	"strings"
)

// folderCountsChunkSize keeps SQL variable counts well under SQLite's limit.
const folderCountsChunkSize = 500

// FolderCountsForConversations counts, per account, how many of the given
// conversations have locally indexed messages in each folder. It backs the
// folder panel's queue filters: queue refs carry only account+conversation
// IDs, so folder membership has to be resolved through the message index.
// The conversation ID mirrors the store's COALESCE(thread_id, id) identity:
// provider thread IDs match messages.thread_id, local single-message IDs
// match messages.id. Conversations with no locally synced messages
// contribute nothing. A conversation spanning folders counts once in each.
func (s *Store) FolderCountsForConversations(accountConversations map[string][]string) (map[string]map[string]int, error) {
	counts := map[string]map[string]int{}
	for accountID, conversationIDs := range accountConversations {
		for start := 0; start < len(conversationIDs); start += folderCountsChunkSize {
			end := start + folderCountsChunkSize
			if end > len(conversationIDs) {
				end = len(conversationIDs)
			}
			if err := s.countConversationFolders(accountID, conversationIDs[start:end], counts); err != nil {
				return nil, err
			}
		}
	}
	return counts, nil
}

func (s *Store) countConversationFolders(accountID string, conversationIDs []string, counts map[string]map[string]int) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(conversationIDs)), ",")
	args := make([]interface{}, 0, 2+2*len(conversationIDs))
	args = append(args, accountID)
	for _, id := range conversationIDs {
		args = append(args, id)
	}
	args = append(args, accountID)
	for _, id := range conversationIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(context.Background(), `
		SELECT folder_id, COUNT(DISTINCT conv) FROM (
			SELECT folder_id, thread_id AS conv FROM messages
			WHERE account_id = ? AND thread_id IS NOT NULL AND thread_id IN (`+placeholders+`)
			UNION ALL
			SELECT folder_id, id FROM messages
			WHERE account_id = ? AND thread_id IS NULL AND id IN (`+placeholders+`)
		)
		GROUP BY folder_id`, args...)
	if err != nil {
		return fmt.Errorf("count queued conversations per folder: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var folderID string
		var count int
		if err := rows.Scan(&folderID, &count); err != nil {
			return err
		}
		if counts[accountID] == nil {
			counts[accountID] = map[string]int{}
		}
		counts[accountID][folderID] += count
	}
	return rows.Err()
}
