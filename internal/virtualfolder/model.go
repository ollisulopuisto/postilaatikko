package virtualfolder

import (
	"time"
)

// VirtualFolder represents a saved set of folders across accounts.
type VirtualFolder struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	OrderIndex int        `json:"orderIndex"`
	FolderIDs  []string   `json:"folderIds"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
