// Package mailfilter provides local cross-account filter rules that match
// incoming messages and route them to a folder on another account.
package mailfilter

import (
	"time"
)

// Rule is a local cross-account routing rule. A message is routed when the
// rule is enabled, the message is on a different account than the
// destination folder, any account/folder scope holds, and every non-empty
// criterion list contains at least one pattern that matches.
type Rule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// OrderIndex is the priority position: lower values are evaluated first
	// and the first matching rule wins.
	OrderIndex int `json:"orderIndex"`

	// Source scope. Empty AccountID matches any account; empty FolderIDs
	// matches any folder on the matched account.
	AccountID string   `json:"accountId"`
	FolderIDs []string `json:"folderIds"`

	// Match criteria. Each non-empty list requires at least one of its
	// case-insensitive substring patterns to match.
	FromContains    []string `json:"fromContains"`
	ToContains      []string `json:"toContains"`
	SubjectContains []string `json:"subjectContains"`

	// DestFolderID is the folder (on another account) the message is moved to.
	DestFolderID string `json:"destFolderId"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Message is the header data a rule is matched against. It mirrors the
// persisted envelope fields so matching stays a pure function of local data.
type Message struct {
	AccountID string
	FolderID  string
	FromName  string
	FromEmail string
	ToNames   []string
	ToEmails  []string
	Subject   string
}
