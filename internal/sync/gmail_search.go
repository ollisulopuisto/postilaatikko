package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
)

const (
	// gmailBlockDuration is how long an account skips the Gmail API after a
	// capability error (scope missing, API disabled) before trying again.
	gmailBlockDuration = 30 * time.Minute

	// gmailMessageIDChunk bounds the number of Message-ID clauses in one IMAP
	// SEARCH command.
	gmailMessageIDChunk = 20

	// gmailDefaultLimit is used when the caller asks for no limit.
	gmailDefaultLimit = 500
)

// GmailTokenSource returns a bearer token for the Gmail API for an account. It
// must return gmail.ErrNotCapable for accounts that cannot use the API (not a
// Google OAuth account, or granted scopes that do not cover search), and
// refresh the token unconditionally when forceRefresh is set.
type GmailTokenSource func(ctx context.Context, accountID string, forceRefresh bool) (string, error)

// SetGmailTokenSource enables Gmail API search for accounts the source accepts.
func (e *Engine) SetGmailTokenSource(src GmailTokenSource) {
	e.gmailTokens = src
}

// GmailSearch searches a Gmail folder with Gmail's own query syntax (messages.list)
// and returns results in the same shape as IMAPSearch. Matching messages are
// mapped to IMAP UIDs by Message-ID header so local data and envelopes can be
// reused. Callers fall back to IMAPSearch on any error except gmail.ErrReauthRequired.
func (e *Engine) GmailSearch(ctx context.Context, accountID, folderID, query string, limit int) (*IMAPSearchResponse, error) {
	if e.accountStore != nil {
		c, apiErr := e.GmailClient(ctx, accountID)
		if apiErr == nil {
			f, err := e.folderStore.Get(folderID)
			if err != nil || f == nil || f.AccountID != accountID {
				return nil, fmt.Errorf("folder account mismatch")
			}
			label, err := e.GmailLabelID(ctx, c, f)
			if err != nil {
				return nil, err
			}
			labels := []string{}
			if label != "" {
				labels = append(labels, label)
			}
			q := query
			if f.Type == folder.TypeArchive {
				q = buildGmailQuery(query, "-in:inbox -in:spam -in:trash")
			}
			page, err := c.Search(ctx, gmail.SearchRequest{Query: q, LabelIDs: labels, Limit: limit})
			if err != nil {
				return nil, err
			}
			headers, err := c.Metadata(ctx, page.MessageIDs)
			if err != nil {
				return nil, err
			}
			local, err := e.cacheGmailMetadata(accountID, folderID, headers)
			if err != nil {
				return nil, err
			}
			out := &IMAPSearchResponse{TotalCount: page.EstimateTotal}
			for _, m := range local {
				out.Results = append(out.Results, &IMAPSearchResult{IsLocal: true, MessageID: m.ID, AccountID: accountID, FolderID: folderID, FolderName: f.Name, Subject: m.Subject, FromName: m.FromName, FromEmail: m.FromEmail, Date: m.Date, Snippet: m.Snippet, IsRead: m.IsRead, IsStarred: m.IsStarred})
			}
			return out, nil
		}
		if !errors.Is(apiErr, gmail.ErrNotCapable) {
			return nil, apiErr
		}
	}

	if e.gmailTokens == nil || e.gmailIsBlocked(accountID) {
		return nil, gmail.ErrNotCapable
	}

	f, err := e.folderStore.Get(folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	if f == nil {
		return nil, fmt.Errorf("folder not found: %s", folderID)
	}
	if f.AccountID != accountID {
		return nil, fmt.Errorf("folder belongs to another account")
	}

	rfcIDs, total, err := e.gmailRFCIDs(ctx, accountID, f, query, limit)
	e.recordGmailState(accountID, err)
	if err != nil {
		e.noteGmailError(accountID, err)
		return nil, err
	}
	if len(rfcIDs) == 0 {
		return &IMAPSearchResponse{TotalCount: 0}, nil
	}

	conn, err := e.pool.GetConnection(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	defer func() { e.pool.Release(conn) }()

	if _, err := conn.Client().SelectMailbox(ctx, f.Path); err != nil {
		return nil, fmt.Errorf("failed to select mailbox: %w", err)
	}
	client := conn.Client().RawClient()

	seen := map[uint32]bool{}
	var uids []uint32
	for start := 0; start < len(rfcIDs); start += gmailMessageIDChunk {
		end := start + gmailMessageIDChunk
		if end > len(rfcIDs) {
			end = len(rfcIDs)
		}
		found, err := uidSearch(ctx, client, messageIDCriteria(rfcIDs[start:end]))
		if err != nil {
			return nil, err
		}
		for _, uid := range found {
			if !seen[uid] {
				seen[uid] = true
				uids = append(uids, uid)
			}
		}
	}
	if len(uids) == 0 {
		return &IMAPSearchResponse{TotalCount: 0}, nil
	}
	if total < len(uids) {
		total = len(uids)
	}
	return e.resolveSearchUIDs(ctx, client, f, accountID, folderID, uids, total, limit)
}

// gmailRFCIDs runs the Gmail search for a folder and returns the RFC Message-ID
// headers of the matches (newest first) plus the API's total estimate.
func (e *Engine) gmailRFCIDs(ctx context.Context, accountID string, f *folder.Folder, query string, limit int) ([]string, int, error) {
	labels, clause, err := gmailFolderScope(f)
	if err != nil {
		return nil, 0, err
	}
	client := gmail.NewClient(func(ctx context.Context, force bool) (string, error) {
		return e.gmailTokens(ctx, accountID, force)
	})
	if e.gmailBaseURL != "" {
		client.BaseURL = e.gmailBaseURL
	}

	want := limit
	if want <= 0 {
		want = gmailDefaultLimit
	}
	q := buildGmailQuery(query, clause)

	var gmailIDs []string
	total := 0
	pageToken := ""
	for pages := 0; len(gmailIDs) < want && pages < 5; pages++ {
		page, err := client.Search(ctx, gmail.SearchRequest{Query: q, LabelIDs: labels, PageToken: pageToken, Limit: want - len(gmailIDs)})
		if err != nil {
			return nil, 0, err
		}
		gmailIDs = append(gmailIDs, page.MessageIDs...)
		total = page.EstimateTotal
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	if len(gmailIDs) == 0 {
		return nil, total, nil
	}

	msgs, err := client.Metadata(ctx, gmailIDs)
	if err != nil {
		return nil, 0, err
	}
	var rfcIDs []string
	for _, m := range msgs {
		if m.RFCMessageID != "" {
			rfcIDs = append(rfcIDs, m.RFCMessageID)
		}
	}
	if total < len(rfcIDs) {
		total = len(rfcIDs)
	}
	return rfcIDs, total, nil
}

// gmailFolderScope maps an IMAP folder of a Gmail account to the label or
// search clause that limits a Gmail search to it. Folders without a known
// mapping return gmail.ErrNotCapable so the caller falls back to IMAP.
func gmailFolderScope(f *folder.Folder) (labelIDs []string, clause string, err error) {
	switch f.Type {
	case folder.TypeInbox:
		return []string{"INBOX"}, "", nil
	case folder.TypeSent:
		return []string{"SENT"}, "", nil
	case folder.TypeDrafts:
		return []string{"DRAFT"}, "", nil
	case folder.TypeTrash:
		return []string{"TRASH"}, "", nil
	case folder.TypeSpam:
		return []string{"SPAM"}, "", nil
	case folder.TypeStarred:
		return []string{"STARRED"}, "", nil
	case folder.TypeAll, folder.TypeArchive:
		return nil, "", nil
	}
	if strings.HasPrefix(f.Path, "[Gmail]/") || f.Path == "" {
		return nil, "", gmail.ErrNotCapable
	}
	// Gmail's label: operator lowercases and replaces spaces with hyphens;
	// nested labels keep their "/" separator.
	token := strings.NewReplacer(" ", "-", `"`, "", "(", "", ")", "", "{", "", "}", "").Replace(strings.ToLower(f.Path))
	return nil, "label:" + token, nil
}

// buildGmailQuery wraps the user's query so operators like OR apply to the
// whole expression, then appends the folder clause.
func buildGmailQuery(userQuery, clause string) string {
	q := "(" + strings.TrimSpace(userQuery) + ")"
	if clause != "" {
		q += " " + clause
	}
	return q
}

// messageIDCriteria builds an IMAP search matching any of the given RFC
// Message-IDs. Angle brackets are stripped for a substring match.
func messageIDCriteria(rfcIDs []string) *imap.SearchCriteria {
	header := func(id string) imap.SearchCriteria {
		return imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Message-ID", Value: strings.Trim(strings.TrimSpace(id), "<>")}}}
	}
	if len(rfcIDs) == 1 {
		c := header(rfcIDs[0])
		return &c
	}
	rest := messageIDCriteria(rfcIDs[1:])
	return &imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{header(rfcIDs[0]), *rest}}}
}

func (e *Engine) gmailIsBlocked(accountID string) bool {
	e.gmailBlockMu.Lock()
	defer e.gmailBlockMu.Unlock()
	until, ok := e.gmailBlocked[accountID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(e.gmailBlocked, accountID)
		return false
	}
	return true
}

// noteGmailError blocks the account from the Gmail API for a while when the
// error says it cannot use it (missing scope, API disabled). Other errors,
// including rate limits and a plain "not capable", leave it enabled.
func (e *Engine) noteGmailError(accountID string, err error) {
	if !errors.Is(err, gmail.ErrInsufficientScope) && !errors.Is(err, gmail.ErrAPIDisabled) {
		return
	}
	e.gmailBlockMu.Lock()
	defer e.gmailBlockMu.Unlock()
	e.gmailBlocked[accountID] = time.Now().Add(gmailBlockDuration)
}
