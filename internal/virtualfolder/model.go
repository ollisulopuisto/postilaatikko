package virtualfolder

import (
	"fmt"
	"strings"
	"time"
)

const (
	FolderModeInclude = "include"
	FolderModeExclude = "exclude"

	ReadStatusAll    = "all"
	ReadStatusUnread = "unread"
	ReadStatusRead   = "read"

	ReplyStatusAll       = "all"
	ReplyStatusReplied   = "replied"
	ReplyStatusUnreplied = "unreplied"

	StarStatusAll       = "all"
	StarStatusStarred   = "starred"
	StarStatusUnstarred = "unstarred"

	DateWindowAll   = "all"
	DateWindow7Day  = "7d"
	DateWindow30Day = "30d"
	DateWindow90Day = "90d"
	DateWindow1Year = "1y"
)

// VirtualFolder represents a saved set of folders across accounts.
type VirtualFolder struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	OrderIndex  int      `json:"orderIndex"`
	FolderIDs   []string `json:"folderIds"`
	FolderMode  string   `json:"folderMode"`
	ReadStatus  string   `json:"readStatus"`
	ReplyStatus string   `json:"replyStatus"`
	StarStatus  string   `json:"starStatus"`
	DateWindow  string   `json:"dateWindow"`
	// SearchQuery is an optional saved text search. When set, the virtual
	// folder lists conversations matching the folder/criteria set AND the query.
	SearchQuery string    `json:"searchQuery"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (vf *VirtualFolder) normalizeCriteria() error {
	if vf.FolderMode == "" {
		vf.FolderMode = FolderModeInclude
	}
	if vf.ReadStatus == "" {
		vf.ReadStatus = ReadStatusAll
	}
	if vf.ReplyStatus == "" {
		vf.ReplyStatus = ReplyStatusAll
	}
	if vf.StarStatus == "" {
		vf.StarStatus = StarStatusAll
	}
	if vf.DateWindow == "" {
		vf.DateWindow = DateWindowAll
	}
	vf.SearchQuery = strings.TrimSpace(vf.SearchQuery)
	if vf.FolderMode != FolderModeInclude && vf.FolderMode != FolderModeExclude {
		return fmt.Errorf("invalid virtual folder mode: %s", vf.FolderMode)
	}
	if vf.ReadStatus != ReadStatusAll && vf.ReadStatus != ReadStatusUnread && vf.ReadStatus != ReadStatusRead {
		return fmt.Errorf("invalid virtual folder read status: %s", vf.ReadStatus)
	}
	if vf.ReplyStatus != ReplyStatusAll && vf.ReplyStatus != ReplyStatusReplied && vf.ReplyStatus != ReplyStatusUnreplied {
		return fmt.Errorf("invalid virtual folder reply status: %s", vf.ReplyStatus)
	}
	if vf.StarStatus != StarStatusAll && vf.StarStatus != StarStatusStarred && vf.StarStatus != StarStatusUnstarred {
		return fmt.Errorf("invalid virtual folder star status: %s", vf.StarStatus)
	}
	if vf.DateWindow != DateWindowAll && vf.DateWindow != DateWindow7Day && vf.DateWindow != DateWindow30Day && vf.DateWindow != DateWindow90Day && vf.DateWindow != DateWindow1Year {
		return fmt.Errorf("invalid virtual folder date window: %s", vf.DateWindow)
	}
	if vf.FolderIDs == nil {
		vf.FolderIDs = []string{}
	}
	return nil
}
