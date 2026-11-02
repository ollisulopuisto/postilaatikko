package mailfilter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/message"
)

// MoveHistory stores bounded header-only samples from explicit user moves.
type MoveHistory struct{ db *database.DB }

// SuggestedFilter is an editable draft derived from repeated independent moves.
type SuggestedFilter struct {
	Key                 string   `json:"key"`
	AccountID           string   `json:"accountId"`
	SourceFolderID      string   `json:"sourceFolderId"`
	DestFolderID        string   `json:"destFolderId"`
	DestAccountID       string   `json:"destAccountId"`
	FromContains        []string `json:"fromContains"`
	ToContains          []string `json:"toContains"`
	SubjectContains     []string `json:"subjectContains"`
	RequiresGmailFilter bool     `json:"requiresGmailFilter"`
}

// NewMoveHistory creates a store for local move samples.
func NewMoveHistory(db *database.DB) *MoveHistory { return &MoveHistory{db: db} }

// Record saves only envelope headers for one explicit move action.
func (s *MoveHistory) Record(actionID, destFolderID string, messages []*message.Message) error {
	if actionID == "" || destFolderID == "" {
		return fmt.Errorf("action and destination are required")
	}
	if len(messages) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range messages {
		if m == nil || m.FolderID == "" || m.FolderID == destFolderID {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO mail_filter_move_history(action_id,account_id,folder_id,dest_folder_id,from_name,from_email,to_list,cc_list,subject,moved_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, actionID, m.AccountID, m.FolderID, destFolderID, m.FromName, m.FromEmail, m.ToList, m.CcList, m.Subject, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM mail_filter_move_history WHERE id NOT IN (SELECT id FROM mail_filter_move_history ORDER BY moved_at DESC,id DESC LIMIT 1000)`); err != nil {
		return err
	}
	return tx.Commit()
}

// SuggestionForRecentMove returns an existing or newly-derived suggestion when
// a second independent action moves a matching envelope to the same folder.
func (s *MoveHistory) SuggestionForRecentMove(actionID, accountID, destFolderID string) (*SuggestedFilter, error) {
	rows, err := s.db.Query(`SELECT action_id,folder_id,from_email,to_list,cc_list,subject FROM mail_filter_move_history WHERE account_id=? AND dest_folder_id=? AND action_id<>? ORDER BY moved_at DESC LIMIT 100`, accountID, destFolderID, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []moveSample
	for rows.Next() {
		var v moveSample
		if err := rows.Scan(&v.action, &v.folder, &v.from, &v.to, &v.cc, &v.subject); err != nil {
			return nil, err
		}
		samples = append(samples, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var current moveSample
	if err := s.db.QueryRow(`SELECT folder_id,from_email,to_list,cc_list,subject FROM mail_filter_move_history WHERE account_id=? AND dest_folder_id=? AND action_id=? LIMIT 1`, accountID, destFolderID, actionID).Scan(&current.folder, &current.from, &current.to, &current.cc, &current.subject); err != nil {
		return nil, err
	}
	for _, prior := range samples {
		if prior.folder != current.folder {
			continue
		}
		from := sharedExact(prior.from, current.from)
		to := sharedAddresses(prior.to+" "+prior.cc, current.to+" "+current.cc)
		subject := sharedSubject(prior.subject, current.subject)
		if len(from)+len(to)+len(subject) == 0 {
			continue
		}
		keyMaterial := strings.Join([]string{accountID, current.folder, destFolderID, strings.Join(from, "|"), strings.Join(to, "|"), strings.Join(subject, "|")}, "|")
		key := uuid.NewSHA1(uuid.NameSpaceURL, []byte(keyMaterial)).String()
		draft := &SuggestedFilter{Key: key, AccountID: accountID, SourceFolderID: current.folder, DestFolderID: destFolderID, FromContains: from, ToContains: to, SubjectContains: subject}
		payload, _ := json.Marshal(draft)
		_, err := s.db.Exec(`INSERT OR IGNORE INTO mail_filter_suggestions(suggestion_key,state,payload_json,updated_at) VALUES(?,?,?,?)`, key, "pending", string(payload), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return nil, err
		}
		var state string
		if err := s.db.QueryRow(`SELECT state FROM mail_filter_suggestions WHERE suggestion_key=?`, key).Scan(&state); err != nil {
			return nil, err
		}
		if state != "pending" {
			continue
		}
		return draft, nil
	}
	return nil, nil
}

func (s *MoveHistory) Pending() ([]*SuggestedFilter, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM mail_filter_suggestions WHERE state='pending' ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SuggestedFilter
	for rows.Next() {
		var raw string
		var v SuggestedFilter
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// GetPending returns a suggestion only while it remains in the pending state.
func (s *MoveHistory) GetPending(key string) (*SuggestedFilter, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT payload_json FROM mail_filter_suggestions WHERE suggestion_key=? AND state='pending'`, key).Scan(&raw); err != nil {
		return nil, err
	}
	var suggestion SuggestedFilter
	if err := json.Unmarshal([]byte(raw), &suggestion); err != nil {
		return nil, err
	}
	return &suggestion, nil
}

// SetState suppresses a pending suggestion after dismissal or creation.
func (s *MoveHistory) SetState(key, state string) error {
	if state != "dismissed" && state != "created" {
		return fmt.Errorf("invalid suggestion state")
	}
	_, err := s.db.Exec(`UPDATE mail_filter_suggestions SET state=?,updated_at=? WHERE suggestion_key=?`, state, time.Now().UTC().Format(time.RFC3339Nano), key)
	return err
}

type moveSample struct{ action, folder, from, to, cc, subject string }

func sharedExact(a, b string) []string {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a != "" && a == b {
		return []string{a}
	}
	return nil
}
func sharedAddresses(a, b string) []string {
	aa := addresses(a)
	bb := addresses(b)
	for _, v := range aa {
		if v != "" && containsFold(bb, v) {
			return []string{v}
		}
	}
	return nil
}
func addresses(raw string) []string {
	var vals []message.Address
	_ = json.Unmarshal([]byte(raw), &vals)
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		e := strings.ToLower(strings.TrimSpace(v.Email))
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}
func sharedSubject(a, b string) []string {
	left := subjectWords(a)
	right := subjectWords(b)
	var common []string
	for _, word := range left {
		if containsFold(right, word) {
			common = append(common, word)
			if len(common) == 2 {
				break
			}
		}
	}
	return common
}
func subjectWords(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= '\u0080') })
	keep := make([]string, 0, len(words))
	for _, w := range words {
		if len([]rune(w)) >= 4 && !isGenericSubjectWord(w) && !containsFold(keep, w) {
			keep = append(keep, w)
		}
	}
	return keep
}

func isGenericSubjectWord(word string) bool {
	switch word {
	case "your", "from", "with", "hello", "this", "that", "have", "will", "weekly", "update", "notification", "message":
		return true
	default:
		return false
	}
}
