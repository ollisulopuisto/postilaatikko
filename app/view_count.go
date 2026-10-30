package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/virtualfolder"
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
	cancel      context.CancelFunc
	state       VirtualFolderOnlineCount
	key         string
	subscribers int
	views       map[string]string
	created     time.Time
	invalidated bool
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
	key := onlineCountKey(vf, accounts, scope)
	job, ctx, fresh, err := a.acquireViewCount(sessionID, virtualFolderID, key)
	if err != nil || !fresh {
		return err
	}
	cancel := job.cancel
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
				count, countErr = client.CountThreads(ctx, gmail.CountQuery{MembershipQuery: gmailViewQuery(vf.SearchQuery, scope[acc.ID], vf.FolderMode == "exclude"), ReadStatus: vf.ReadStatus, StarStatus: vf.StarStatus, ReplyStatus: vf.ReplyStatus, Cutoff: cutoff}, func(progress gmail.ThreadCount) {
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
		job.state.Complete = len(job.state.Issues) == 0 && ctx.Err() == nil && !job.invalidated
		a.viewCountMu.Unlock()
	}()
	return nil
}

// GetVirtualFolderOnlineCount returns an isolated snapshot of the window count job.
func (a *App) GetVirtualFolderOnlineCount(sessionID string) (VirtualFolderOnlineCount, error) {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	job := a.viewCountJobs[sessionID]
	if job == nil {
		return VirtualFolderOnlineCount{}, fmt.Errorf("count session not found")
	}
	state := job.state
	state.SessionID = sessionID
	state.VirtualFolderID = job.views[sessionID]
	state.Issues = append([]MailSearchIssue{}, state.Issues...)
	return state, nil
}

// CancelVirtualFolderOnlineCount also releases completed session state. Counts
// are owned by the requesting window; closing it must not leave background work.
func (a *App) CancelVirtualFolderOnlineCount(sessionID string) {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	a.detachViewCount(sessionID)
}

// Keys contain only provider/account identity and effective criteria, not credentials.
func onlineCountKey(vf *virtualfolder.VirtualFolder, accounts []*account.Account, scope map[string][]*folder.Folder) string {
	parts := []string{vf.FolderMode, vf.SearchQuery, vf.ReadStatus, vf.StarStatus, vf.ReplyStatus, vf.DateWindow}
	identities := []string{}
	for _, acc := range accounts {
		if !acc.Enabled || vf.FolderMode != "exclude" && len(scope[acc.ID]) == 0 {
			continue
		}
		identity, _ := json.Marshal([]string{acc.ID, acc.Email, acc.IMAPHost, acc.Username, string(acc.AuthType)})
		identities = append(identities, string(identity))
		for _, f := range scope[acc.ID] {
			member, _ := json.Marshal([]string{acc.ID, f.ID, f.Path, string(f.Type)})
			identities = append(identities, string(member))
		}
	}
	sort.Strings(identities)
	encoded, _ := json.Marshal(append(parts, identities...))
	return string(encoded)
}

func (a *App) acquireViewCount(sessionID, viewID, key string) (*viewCountJob, context.Context, bool, error) {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	if a.viewCountJobs == nil {
		a.viewCountJobs = map[string]*viewCountJob{}
	}
	if a.viewCountShared == nil {
		a.viewCountShared = map[string]*viewCountJob{}
	}
	if len(a.viewCountJobs) >= 32 && a.viewCountJobs[sessionID] == nil {
		return nil, nil, false, fmt.Errorf("too many count sessions")
	}
	a.detachViewCount(sessionID)
	now := time.Now()
	for key, job := range a.viewCountShared {
		if now.Sub(job.created) >= time.Minute && !job.state.Running {
			delete(a.viewCountShared, key)
		}
	}
	if job := a.viewCountShared[key]; job != nil && !job.invalidated && (job.state.Running || job.state.Complete && now.Sub(job.created) < time.Minute) {
		job.subscribers++
		job.views[sessionID] = viewID
		a.viewCountJobs[sessionID] = job
		return job, nil, false, nil
	}
	// Limit both active jobs and retained completed results.
	if len(a.viewCountShared) >= 64 {
		for key, job := range a.viewCountShared {
			if !job.state.Running {
				delete(a.viewCountShared, key)
				break
			}
		}
	}
	if len(a.viewCountShared) >= 64 {
		return nil, nil, false, fmt.Errorf("too many shared counts")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	job := &viewCountJob{cancel: cancel, key: key, subscribers: 1, views: map[string]string{}, created: now, state: VirtualFolderOnlineCount{VirtualFolderID: viewID, Running: true, Issues: []MailSearchIssue{}}}
	a.viewCountShared[key] = job
	job.views[sessionID] = viewID
	a.viewCountJobs[sessionID] = job
	return job, ctx, true, nil
}

// Caller holds viewCountMu. Only the last subscriber may cancel active work.
func (a *App) detachViewCount(sessionID string) {
	if job := a.viewCountJobs[sessionID]; job != nil {
		delete(a.viewCountJobs, sessionID)
		delete(job.views, sessionID)
		job.subscribers--
		if job.subscribers == 0 && job.state.Running {
			job.cancel()
			if a.viewCountShared[job.key] == job {
				delete(a.viewCountShared, job.key)
			}
		}
	}
}

// Invalidation also makes subscribed snapshots non-exact until the next scan.
func (a *App) invalidateViewCounts() {
	a.viewCountMu.Lock()
	defer a.viewCountMu.Unlock()
	for _, job := range a.viewCountJobs {
		job.invalidated = true
		job.state.Complete = false
		job.cancel()
	}
	for key, job := range a.viewCountShared {
		job.invalidated = true
		job.state.Complete = false
		job.cancel()
		delete(a.viewCountShared, key)
	}
}
