// Package mailservice contains provider-aware mailbox operations shared by
// the desktop application and the headless service.
package mailservice

import (
	"strings"
	"sync"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/email/replycontent"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/replylater"
	mailSync "github.com/ollisulopuisto/postilaatikko/internal/sync"
	"github.com/ollisulopuisto/postilaatikko/internal/triage"
	"github.com/ollisulopuisto/postilaatikko/internal/virtualfolder"
)

// Options supplies the stores and provider engine already owned by a runtime.
type Options struct {
	Accounts       *account.Store
	Folders        *folder.Store
	Messages       *message.Store
	ReplyLater     *replylater.Store
	Triage         *triage.Store
	VirtualFolders *virtualfolder.Store
	Sync           *mailSync.Engine
	ReplyContent   *replycontent.Service
	SyncTriage     func(queue string)
}

// Service implements provider-aware search using caller-owned mailbox state.
type Service struct {
	accountStore       *account.Store
	folderStore        *folder.Store
	messageStore       *message.Store
	replyLaterStore    *replylater.Store
	triageStore        *triage.Store
	virtualFolderStore *virtualfolder.Store
	syncEngine         *mailSync.Engine
	replyContent       *replycontent.Service
	syncTriageQueues   func(string)
	mailSearchMu       sync.Mutex
	mailSearchJobs     map[string]*mailSearchJob
}

// New constructs a service around stores and a sync engine owned elsewhere.
func New(options Options) *Service {
	return &Service{
		accountStore:       options.Accounts,
		folderStore:        options.Folders,
		messageStore:       options.Messages,
		replyLaterStore:    options.ReplyLater,
		triageStore:        options.Triage,
		virtualFolderStore: options.VirtualFolders,
		syncEngine:         options.Sync,
		replyContent:       options.ReplyContent,
		syncTriageQueues:   options.SyncTriage,
	}
}

func combineVirtualFolderQueries(stored, typed string) string {
	return strings.TrimSpace(strings.TrimSpace(stored) + " " + strings.TrimSpace(typed))
}
