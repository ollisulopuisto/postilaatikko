package app

import (
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailservice"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

type MailSearchRequest = mailservice.MailSearchRequest
type MailSearchIssue = mailservice.MailSearchIssue
type MailSearchPage = mailservice.MailSearchPage

func gmailViewQuery(query string, folders []*folder.Folder, exclude bool) string {
	return mailservice.GmailViewQuery(query, folders, exclude)
}

func matchesGmailThread(items []gmail.Message, vf *virtualfolder.VirtualFolder) bool {
	return mailservice.MatchesGmailThread(items, vf)
}

func (a *App) currentMailService() *mailservice.Service {
	a.mailServiceMu.Lock()
	defer a.mailServiceMu.Unlock()
	if a.mailService == nil {
		a.mailService = mailservice.New(mailservice.Options{
			Accounts:       a.accountStore,
			Folders:        a.folderStore,
			Messages:       a.messageStore,
			ReplyLater:     a.replyLaterStore,
			Triage:         a.triageStore,
			VirtualFolders: a.virtualFolderStore,
			Sync:           a.syncEngine,
			ReplyContent:   a.replyContent,
			SyncTriage:     a.syncGmailTriageQueues,
		})
	}
	return a.mailService
}

// SearchMail keeps the desktop API stable while sharing its provider-aware
// search operation with the headless service.
func (a *App) SearchMail(req MailSearchRequest) (*MailSearchPage, error) {
	return a.currentMailService().Search(a.ctx, req)
}

// CancelMailSearch cancels a search by its UI session identifier.
func (a *App) CancelMailSearch(sessionID string) {
	a.currentMailService().Cancel(sessionID)
}
