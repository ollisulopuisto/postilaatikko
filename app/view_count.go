package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
)

// VirtualFolderOnlineCount distinguishes full server counts from partial work.
// An errored/cancelled account can never contribute an allegedly exact total.
type VirtualFolderOnlineCount struct {
	SessionID       string            `json:"sessionId"`
	VirtualFolderID string            `json:"virtualFolderId"`
	Total           int               `json:"total"`
	Unread          int               `json:"unread"`
	Scanned         int               `json:"scanned"`
	Running         bool              `json:"running"`
	Complete        bool              `json:"complete"`
	Issues          []MailSearchIssue `json:"issues"`
}
type viewCountJob struct {
	cancel context.CancelFunc
	state  VirtualFolderOnlineCount
}

// StartVirtualFolderOnlineCount returns immediately; read progress with
// GetVirtualFolderOnlineCount. Reusing a window's session cancels its old work.
func (a *App) StartVirtualFolderOnlineCount(sessionID, virtualFolderID string) error {
	if sessionID == "" || len(sessionID) > 128 {
		return fmt.Errorf("invalid count session")
	}
	vf, err := a.virtualFolderStore.Get(virtualFolderID)
	if err != nil {
		return err
	}
	if vf == nil {
		return fmt.Errorf("virtual folder not found")
	}
	accounts, err := a.accountStore.List()
	if err != nil {
		return err
	}
	scope := map[string][]*folder.Folder{}
	for _, id := range vf.FolderIDs {
		f, err := a.folderStore.Get(id)
		if err != nil {
			return err
		}
		if f != nil {
			scope[f.AccountID] = append(scope[f.AccountID], f)
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	job := &viewCountJob{cancel: cancel, state: VirtualFolderOnlineCount{SessionID: sessionID, VirtualFolderID: virtualFolderID, Running: true, Issues: []MailSearchIssue{}}}
	a.viewCountMu.Lock()
	if a.viewCountJobs == nil {
		a.viewCountJobs = map[string]*viewCountJob{}
	}
	if old := a.viewCountJobs[sessionID]; old != nil {
		old.cancel()
	}
	if len(a.viewCountJobs) >= 32 && a.viewCountJobs[sessionID] == nil {
		a.viewCountMu.Unlock()
		cancel()
		return fmt.Errorf("too many count sessions")
	}
	a.viewCountJobs[sessionID] = job
	a.viewCountMu.Unlock()
	cutoff := time.Time{}
	if days := map[string]int{"7d": 7, "30d": 30, "90d": 90, "1y": 365}[vf.DateWindow]; days > 0 {
		cutoff = time.Now().AddDate(0, 0, -days)
	}
	go func() {
		defer cancel()
		total, unread, scanned := 0, 0, 0
		for _, acc := range accounts {
			if !acc.Enabled || vf.FolderMode != "exclude" && len(scope[acc.ID]) == 0 {
				continue
			}
			client, countErr := a.syncEngine.GmailClient(ctx, acc.ID)
			var count gmail.ThreadCount
			if countErr == nil {
				count, countErr = client.CountThreads(ctx, gmail.CountQuery{MembershipQuery: gmailViewQuery("", scope[acc.ID], vf.FolderMode == "exclude"), ReadStatus: vf.ReadStatus, StarStatus: vf.StarStatus, ReplyStatus: vf.ReplyStatus, Cutoff: cutoff}, func(progress gmail.ThreadCount) {
					a.viewCountMu.Lock()
					job.state.Total = total + progress.Total
					job.state.Unread = unread + progress.Unread
					job.state.Scanned = scanned + progress.Scanned
					a.viewCountMu.Unlock()
				})
			}
			a.viewCountMu.Lock()
			total += count.Total
			unread += count.Unread
			scanned += count.Scanned
			job.state.Total = total
			job.state.Unread = unread
			job.state.Scanned = scanned
			if countErr != nil {
				text := countErr.Error()
				if errors.Is(countErr, gmail.ErrNotCapable) {
					text = "Complete online conversation counts are not yet available for this provider"
				}
				job.state.Issues = append(job.state.Issues, MailSearchIssue{AccountID: acc.ID, AccountName: acc.Name, Message: text, Reauthorize: errors.Is(countErr, gmail.ErrReauthRequired) || errors.Is(countErr, gmail.ErrInsufficientScope)})
			}
			a.viewCountMu.Unlock()
			if ctx.Err() != nil {
				break
			}
		}
		a.viewCountMu.Lock()
		job.state.Running = false
		job.state.Complete = len(job.state.Issues) == 0 && ctx.Err() == nil
		a.viewCountMu.Unlock()
	}()
	return nil
}

func (a *App) GetVirtualFolderOnlineCount(sessionID string) (VirtualFolderOnlineCount, error) {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	job := a.viewCountJobs[sessionID]
	if job == nil {
		return VirtualFolderOnlineCount{}, fmt.Errorf("count session not found")
	}
	state := job.state
	state.Issues = append([]MailSearchIssue{}, state.Issues...)
	return state, nil
}

// CancelVirtualFolderOnlineCount also releases completed session state. Counts
// are owned by the requesting window; closing it must not leave background work.
func (a *App) CancelVirtualFolderOnlineCount(sessionID string) {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	if job := a.viewCountJobs[sessionID]; job != nil {
		job.cancel()
		delete(a.viewCountJobs, sessionID)
	}
}
