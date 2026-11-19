package message

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Keyset pagination for the unified conversation list (PERF-SC-005).
//
// The offset-based list query groups and sorts every message in the selected
// folders on every page, so each page costs O(folder size). The keyset path
// below paginates in two phases:
//
//  1. Phase 1 walks messages newest-first (or oldest-first) per folder using
//     idx_messages_folder_date_conv, deduplicating threads in Go, and stops
//     after limit+1 distinct conversations. The index seek starts at the
//     cursor position, so cost scales with the page, not the skipped rows.
//  2. Phase 2 computes the expensive aggregates (message ids, participants,
//     counts) for only the conversations on the page, looked up through
//     idx_messages_account_conv.
//
// Consistency contract: the cursor names the last conversation of the
// previous page by (latest_date, account_id, thread). Messages inserted or
// moved to a newer position than the cursor after a page was served belong
// to an already-served region and do not shift later pages; they appear on
// the next first-page refresh. Conversations removed between pages are
// skipped. A conversation whose latest message moves across the cursor
// between pages may reappear once; this matches the offset path's behaviour
// under the same mutation.
//
// Filters (unread/starred/...) are evaluated as HAVING clauses over the full
// folder grouping, so filtered lists keep using the offset path until a
// filtered keyset path is designed.

// KeysetCursorPrefix marks an opaque keyset cursor so handlers can tell it
// apart from a legacy numeric offset cursor.
const KeysetCursorPrefix = "k1:"

type conversationCursor struct {
	Date    string `json:"d"` // latest_date of the last conversation on the previous page
	Account string `json:"a"`
	Thread  string `json:"t"`
	Order   string `json:"o"` // "newest" or "oldest"
}

// EncodeConversationCursor renders an opaque cursor for the API boundary.
func EncodeConversationCursor(date, account, thread, order string) string {
	raw, err := json.Marshal(conversationCursor{Date: date, Account: account, Thread: thread, Order: order})
	if err != nil {
		return ""
	}
	return KeysetCursorPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func decodeConversationCursor(cursor string) (conversationCursor, error) {
	var c conversationCursor
	if !strings.HasPrefix(cursor, KeysetCursorPrefix) {
		return c, fmt.Errorf("not a keyset cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, KeysetCursorPrefix))
	if err != nil {
		return c, fmt.Errorf("malformed keyset cursor")
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("malformed keyset cursor")
	}
	if c.Date == "" || c.Thread == "" || (c.Order != "newest" && c.Order != "oldest") {
		return c, fmt.Errorf("incomplete keyset cursor")
	}
	return c, nil
}

// conversationKey identifies one conversation for page assembly.
type conversationKey struct {
	date    string
	account string
	thread  string
	folder  string
}

// ListConversationsByFolderIDsKeyset returns one page of conversations using
// keyset pagination. Pass an empty cursor for the first page. The returned
// nextCursor is empty when the page is the last one. sortOrder is "newest"
// (default) or "oldest". excludeInboxCopies selects the Archive variant that
// hides Gmail messages still present in an Inbox folder.
func (s *Store) ListConversationsByFolderIDsKeyset(folderIDs []string, cursor string, limit int, sortOrder string, excludeInboxCopies bool) ([]*Conversation, string, error) {
	// Phase 1 derives a conversation's sort key from the first message met
	// while scanning newest-first. Oldest-first would need each thread's
	// latest message before ranking, which is the full-folder grouping the
	// keyset path exists to avoid; oldest stays on the offset path.
	if sortOrder != "" && sortOrder != "newest" {
		return nil, "", fmt.Errorf("keyset pagination supports newest ordering only")
	}
	sortOrder = "newest"
	if limit <= 0 {
		return []*Conversation{}, "", nil
	}
	cur := conversationCursor{Order: sortOrder}
	if cursor != "" {
		decoded, err := decodeConversationCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		if decoded.Order != sortOrder {
			return nil, "", fmt.Errorf("keyset cursor sort order mismatch")
		}
		cur = decoded
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	folderAccount, err := s.enabledFolderAccounts(ctx, folderIDs)
	if err != nil {
		return nil, "", err
	}

	keys, hasMore, err := s.collectPageKeys(ctx, folderIDs, folderAccount, cur, limit, sortOrder, excludeInboxCopies)
	if err != nil {
		return nil, "", err
	}
	if len(keys) == 0 {
		return []*Conversation{}, "", nil
	}

	convs, err := s.loadConversationAggregates(ctx, keys, folderAccount, excludeInboxCopies)
	if err != nil {
		return nil, "", err
	}

	byKey := make(map[string]*Conversation, len(convs))
	for _, c := range convs {
		byKey[c.AccountID+"\x00"+c.ThreadID] = c
	}
	page := make([]*Conversation, 0, len(keys))
	for _, k := range keys {
		if c, ok := byKey[k.account+"\x00"+k.thread]; ok {
			page = append(page, c)
		}
	}
	next := ""
	if hasMore {
		last := keys[len(keys)-1]
		next = EncodeConversationCursor(last.date, last.account, last.thread, sortOrder)
	}
	return page, next, nil
}

// enabledFolderAccounts resolves folder ids to their account, skipping
// folders on disabled accounts or folders that no longer exist.
func (s *Store) enabledFolderAccounts(ctx context.Context, folderIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(folderIDs))
	if len(folderIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(folderIDs)), ",")
	args := make([]interface{}, len(folderIDs))
	for i, id := range folderIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT f.id, f.account_id FROM folders f
		INNER JOIN accounts a ON f.account_id = a.id AND a.enabled = 1
		WHERE f.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve folder accounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var fid, aid string
		if err := rows.Scan(&fid, &aid); err != nil {
			return nil, fmt.Errorf("failed to resolve folder accounts: %w", err)
		}
		out[fid] = aid
	}
	return out, rows.Err()
}

// folderKeyIter lazily walks one folder's messages in latest-first (or
// oldest-first) order, exposing the first-seen key of each thread.
type folderKeyIter struct {
	rows    *sql.Rows
	account string
	folder  string
	peeked  *conversationKey
	err     error
	done    bool
}

// collectPageKeys performs phase 1: a k-way merge over per-folder index
// scans, deduplicating threads, returning up to limit keys plus whether at
// least one more conversation exists beyond the page.
func (s *Store) collectPageKeys(ctx context.Context, folderIDs []string, folderAccount map[string]string, cur conversationCursor, limit int, sortOrder string, excludeInboxCopies bool) ([]conversationKey, bool, error) {
	newest := sortOrder != "oldest"
	iters := make([]*folderKeyIter, 0, len(folderIDs))
	for _, fid := range folderIDs {
		account, ok := folderAccount[fid]
		if !ok {
			continue
		}
		it, err := s.newFolderKeyIter(ctx, fid, account, cur, newest, excludeInboxCopies)
		if err != nil {
			return nil, false, err
		}
		iters = append(iters, it)
	}
	defer func() {
		for _, it := range iters {
			if it.rows != nil {
				it.rows.Close()
			}
		}
	}()

	seen := make(map[string]bool)
	keys := make([]conversationKey, 0, limit+1)
	hasMore := false
	for {
		best := -1
		for i, it := range iters {
			k := it.peek()
			if k == nil {
				continue
			}
			if best == -1 || keyLess(*k, *iters[best].peek(), newest) {
				best = i
			}
		}
		if best == -1 {
			break
		}
		k := iters[best].peek()
		iters[best].advance()
		dedupeKey := k.account + "\x00" + k.thread
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true
		if cur.Date != "" && !keyAfterCursor(*k, cur, newest) {
			continue
		}
		if len(keys) == limit {
			hasMore = true
			break
		}
		keys = append(keys, *k)
	}
	for _, it := range iters {
		if it.err != nil {
			return nil, false, fmt.Errorf("failed to walk conversation keys: %w", it.err)
		}
	}
	return keys, hasMore, nil
}

func (s *Store) newFolderKeyIter(ctx context.Context, folderID, account string, cur conversationCursor, newest bool, excludeInboxCopies bool) (*folderKeyIter, error) {
	dateBound := ""
	order := "ORDER BY m.date DESC, COALESCE(m.thread_id, m.id) ASC"
	args := []interface{}{folderID}
	if cur.Date != "" {
		if newest {
			dateBound = " AND m.date <= ?"
		} else {
			dateBound = " AND m.date >= ?"
		}
		args = append(args, cur.Date)
	}
	if !newest {
		order = "ORDER BY m.date ASC, COALESCE(m.thread_id, m.id) ASC"
	}
	query := `SELECT COALESCE(m.thread_id, m.id), m.date
		FROM messages m
		WHERE m.folder_id = ?` + dateBound + conversationVisibilityClauses(excludeInboxCopies) + `
		` + order
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to walk conversation keys: %w", err)
	}
	return &folderKeyIter{rows: rows, account: account, folder: folderID}, nil
}

func (it *folderKeyIter) peek() *conversationKey {
	if it.peeked == nil && !it.done {
		it.advance()
	}
	return it.peeked
}

func (it *folderKeyIter) advance() {
	if it.done {
		it.peeked = nil
		return
	}
	if it.rows.Next() {
		var thread, date string
		if err := it.rows.Scan(&thread, &date); err != nil {
			it.err = err
			it.done = true
			it.peeked = nil
			return
		}
		it.peeked = &conversationKey{date: date, account: it.account, thread: thread, folder: it.folder}
		return
	}
	it.err = it.rows.Err()
	it.done = true
	it.peeked = nil
}

// keyLess orders page candidates: newest first by date, then account and
// thread ascending, matching the offset path's ORDER BY tie-breakers.
func keyLess(a, b conversationKey, newest bool) bool {
	if a.date != b.date {
		if newest {
			return a.date > b.date
		}
		return a.date < b.date
	}
	if a.account != b.account {
		return a.account < b.account
	}
	return a.thread < b.thread
}

// keyAfterCursor reports whether k sorts strictly after the cursor position.
func keyAfterCursor(k conversationKey, cur conversationCursor, newest bool) bool {
	if k.date != cur.Date {
		if newest {
			return k.date < cur.Date
		}
		return k.date > cur.Date
	}
	if k.account != cur.Account {
		return k.account > cur.Account
	}
	return k.thread > cur.Thread
}

// loadConversationAggregates performs phase 2: the full aggregate projection
// for only the conversations on the page, grouped per account.
func (s *Store) loadConversationAggregates(ctx context.Context, keys []conversationKey, folderAccount map[string]string, excludeInboxCopies bool) ([]*Conversation, error) {
	byAccount := make(map[string][]string)
	foldersByAccount := make(map[string][]string)
	for _, k := range keys {
		byAccount[k.account] = appendIfMissing(byAccount[k.account], k.thread)
	}
	for fid, aid := range folderAccount {
		foldersByAccount[aid] = append(foldersByAccount[aid], fid)
	}
	var out []*Conversation
	for account, threads := range byAccount {
		folders := foldersByAccount[account]
		if len(folders) == 0 {
			continue
		}
		sort.Strings(folders)
		threadPlaceholders := strings.TrimRight(strings.Repeat("?,", len(threads)), ",")
		query := buildConversationAggregateQuery(len(folders), excludeInboxCopies) +
			` AND m.account_id = ? AND COALESCE(m.thread_id, m.id) IN (` + threadPlaceholders + `)
		GROUP BY COALESCE(m.thread_id, m.id), a.id`
		// account_id is the first predicate after the folder IN list.
		// Placeholder order: folder IN list, then account, then thread IN list.
		args := make([]interface{}, 0, 1+len(folders)+len(threads))
		for _, f := range folders {
			args = append(args, f)
		}
		args = append(args, account)
		for _, t := range threads {
			args = append(args, t)
		}
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to load conversation page: %w", err)
		}
		convs, err := scanConversations(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, convs...)
	}
	return out, nil
}

func appendIfMissing(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}
