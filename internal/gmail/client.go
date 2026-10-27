// Package gmail is a thin REST client for the Gmail API (users.messages).
// It knows nothing about the UI or the local store: callers supply a token
// source and map results to local messages themselves. See
// docs/plans/2026-10-24-gmail-api-contract.md for the design and scope plan.
package gmail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the Gmail API root for the authenticated user.
	DefaultBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"

	maxAttempts         = 3
	maxSearchResults    = 500
	maxBatchModifyIDs   = 1000
	metadataConcurrency = 8
	maxResponseBytes    = 4 << 20
	baseBackoff         = 500 * time.Millisecond
)

// TokenSource returns a bearer token. With forceRefresh it must obtain a new
// token even if the stored one has not expired (used after a 401).
type TokenSource func(ctx context.Context, forceRefresh bool) (string, error)

// Client calls the Gmail REST API. The zero value is not usable; use NewClient.
type Client struct {
	// BaseURL can be overridden in tests.
	BaseURL    string
	HTTPClient *http.Client
	Budget     *Budget // shared account quota admission; nil for isolated transports

	tokenSource TokenSource
	sleep       func(ctx context.Context, d time.Duration) error
}

// NewClient returns a client that authenticates with tokenSource.
func NewClient(tokenSource TokenSource) *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
		tokenSource: tokenSource,
		sleep:       sleepContext,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// SearchRequest is a messages.list query.
type SearchRequest struct {
	Query     string   // Gmail search syntax
	LabelIDs  []string // restrict to messages carrying all of these labels
	PageToken string
	Limit     int // maxResults, capped at 500

	// IncludeSpamTrash also searches Spam and Trash. It is implied when LabelIDs
	// contains SPAM or TRASH.
	IncludeSpamTrash bool
}

// SearchPage is one page of message ids.
type SearchPage struct {
	MessageIDs    []string
	NextPageToken string
	EstimateTotal int
}

// Search lists message ids matching the request. Spam and trash are only
// included when the query or labels ask for them (includeSpamTrash is set when
// a SPAM or TRASH label is requested).
func (c *Client) Search(ctx context.Context, r SearchRequest) (*SearchPage, error) {
	q := url.Values{}
	if r.Query != "" {
		q.Set("q", r.Query)
	}
	limit := r.Limit
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}
	q.Set("maxResults", strconv.Itoa(limit))
	if r.PageToken != "" {
		q.Set("pageToken", r.PageToken)
	}
	if r.IncludeSpamTrash {
		q.Set("includeSpamTrash", "true")
	}
	for _, l := range r.LabelIDs {
		q.Add("labelIds", l)
		if l == "SPAM" || l == "TRASH" {
			q.Set("includeSpamTrash", "true")
		}
	}

	var resp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken      string `json:"nextPageToken"`
		ResultSizeEstimate int    `json:"resultSizeEstimate"`
	}
	if err := c.do(ctx, http.MethodGet, "/messages", q, nil, &resp); err != nil {
		return nil, err
	}
	page := &SearchPage{NextPageToken: resp.NextPageToken, EstimateTotal: resp.ResultSizeEstimate}
	for _, m := range resp.Messages {
		page.MessageIDs = append(page.MessageIDs, m.ID)
	}
	return page, nil
}

// Message is the metadata subset the client reads.
type Message struct {
	ID                                          string
	ThreadID                                    string
	LabelIDs                                    []string
	Snippet                                     string
	Date                                        time.Time
	To, Cc, Bcc, ReplyTo, InReplyTo, References string
	Headers                                     map[string]string
	Size                                        int
	RFCMessageID                                string // Message-ID header, with angle brackets as sent
	Subject                                     string
	From                                        string
}

// Metadata fetches headers and labels (never bodies) for the given ids with
// bounded concurrency, preserving the input order. Messages that no longer
// exist are skipped.
func (c *Client) Metadata(ctx context.Context, ids []string) ([]Message, error) {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]*Message, len(ids))
	sem := make(chan struct{}, metadataConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
enqueue:
	for i, id := range ids {
		select {
		case sem <- struct{}{}:
		case <-workCtx.Done():
			break enqueue
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := c.metadataOne(workCtx, id)
			if err != nil && !isNotFound(err) {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
				return
			}
			results[i] = m
		}(i, id)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(ids))
	for _, m := range results {
		if m != nil {
			out = append(out, *m)
		}
	}
	return out, nil
}

func isNotFound(err error) bool {
	ae, ok := err.(*APIError)
	return ok && ae.kind == ErrNotFound
}

func (c *Client) metadataOne(ctx context.Context, id string) (*Message, error) {
	q := url.Values{}
	q.Set("format", "metadata")
	for _, h := range metadataHeaders {
		q.Add("metadataHeaders", h)
	}
	var resp wireMessage
	if err := c.do(ctx, http.MethodGet, "/messages/"+url.PathEscape(id), q, nil, &resp); err != nil {
		return nil, err
	}
	m := resp.metadata()
	return m, nil
}

// Modify adds and removes labels on the given messages. A single id uses
// messages.modify; several use messages.batchModify in chunks of 1000.
func (c *Client) Modify(ctx context.Context, ids, addLabels, removeLabels []string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		body := map[string]any{"addLabelIds": addLabels, "removeLabelIds": removeLabels}
		return c.do(ctx, http.MethodPost, "/messages/"+url.PathEscape(ids[0])+"/modify", nil, body, nil)
	}
	for start := 0; start < len(ids); start += maxBatchModifyIDs {
		end := start + maxBatchModifyIDs
		if end > len(ids) {
			end = len(ids)
		}
		body := map[string]any{"ids": ids[start:end], "addLabelIds": addLabels, "removeLabelIds": removeLabels}
		if err := c.do(ctx, http.MethodPost, "/messages/batchModify", nil, body, nil); err != nil {
			return err
		}
	}
	return nil
}

// do performs a request with token refresh on 401 and bounded retries for rate
// limits and server errors.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.request(ctx, method, path, query, body, out, true, maxResponseBytes)
}
func (c *Client) request(ctx context.Context, method, path string, query url.Values, body, out any, retry bool, responseLimit int64) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("gmail: encode request: %w", err)
		}
	}
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	force := false
	refreshed := false
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.Budget != nil {
			if c.Budget.Paused() {
				return ErrRateLimited
			}
			if err := c.Budget.Wait(ctx, method, path, query); err != nil {
				return err
			}
		}
		token, err := c.tokenSource(ctx, force)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("gmail: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !retry || attempt >= maxAttempts {
				return fmt.Errorf("%w: %v", ErrTransient, err)
			}
			if err := c.sleep(ctx, backoff(attempt, 0)); err != nil {
				return err
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
		_ = resp.Body.Close()
		if int64(len(data)) > responseLimit {
			return fmt.Errorf("gmail: response exceeds size limit")
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w: reading response: %v", ErrTransient, readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil || len(data) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("gmail: decode response: %w", err)
			}
			return nil
		}

		apiErr := parseAPIError(resp.StatusCode, data)
		if c.Budget != nil && apiErr.kind == ErrRateLimited {
			c.Budget.Cooldown(retryAfter(resp.Header.Get("Retry-After")))
			// Surface the limit once; all account clients now share the pause.
			// Immediate per-request retries would multiply the exhausted workload.
			return apiErr
		}
		if resp.StatusCode == http.StatusUnauthorized {
			if refreshed {
				apiErr.kind = ErrReauthRequired
				return apiErr
			}
			refreshed, force = true, true
			attempt--
			continue
		}
		if !retry || !isRetryable(apiErr) || attempt >= maxAttempts {
			return apiErr
		}
		if err := c.sleep(ctx, backoff(attempt, retryAfter(resp.Header.Get("Retry-After")))); err != nil {
			return err
		}
	}
}

func isRetryable(e *APIError) bool {
	return e.kind == ErrRateLimited || e.kind == ErrTransient
}

func backoff(attempt int, retryAfter time.Duration) time.Duration {
	d := baseBackoff << (attempt - 1)
	d += time.Duration(rand.Int63n(int64(baseBackoff))) //nolint:gosec // jitter only
	if retryAfter > d {
		return retryAfter
	}
	return d
}

func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func parseAPIError(status int, data []byte) *APIError {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &payload)
	e := &APIError{Status: status, Message: payload.Error.Message}
	reasons := []string{}
	for _, r := range payload.Error.Errors {
		reasons = append(reasons, r.Reason)
	}
	for _, r := range payload.Error.Details {
		reasons = append(reasons, r.Reason)
	}
	if len(reasons) > 0 {
		e.Reason = reasons[0]
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	e.kind = classify(status, reasons)
	return e
}

func classify(status int, reasons []string) error {
	has := func(want ...string) bool {
		for _, r := range reasons {
			for _, w := range want {
				if r == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case status == http.StatusUnauthorized:
		return ErrReauthRequired
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusTooManyRequests,
		status == http.StatusForbidden && has("rateLimitExceeded", "userRateLimitExceeded"):
		return ErrRateLimited
	case status == http.StatusForbidden && has("accessNotConfigured", "SERVICE_DISABLED"):
		return ErrAPIDisabled
	case status == http.StatusForbidden && has("insufficientPermissions", "ACCESS_TOKEN_SCOPE_INSUFFICIENT"):
		return ErrInsufficientScope
	case status >= 500:
		return ErrTransient
	case status == http.StatusBadRequest:
		return ErrBadRequest
	}
	return nil
}

// FindByRFCMessageID returns the Gmail id of the single message whose Message-ID
// header matches (Spam and Trash included). ErrNotFound means no match and
// ErrAmbiguous more than one, so callers never act on a guess.
func (c *Client) FindByRFCMessageID(ctx context.Context, rfcMessageID string) (string, error) {
	id := strings.Trim(strings.TrimSpace(rfcMessageID), "<>")
	if id == "" {
		return "", fmt.Errorf("%w: empty Message-ID", ErrNotFound)
	}
	page, err := c.Search(ctx, SearchRequest{Query: "rfc822msgid:" + id, Limit: 2, IncludeSpamTrash: true})
	if err != nil {
		return "", err
	}
	switch len(page.MessageIDs) {
	case 0:
		return "", ErrNotFound
	case 1:
		return page.MessageIDs[0], nil
	}
	return "", ErrAmbiguous
}

// SetSpam moves the messages with the given Message-ID headers to Spam
// (spam=true: add SPAM, remove INBOX) or back to the Inbox (spam=false: add
// INBOX, remove SPAM). Every message is resolved first; if any lookup fails
// nothing is changed, so a failure leaves the mailbox untouched.
func (c *Client) SetSpam(ctx context.Context, rfcMessageIDs []string, spam bool) error {
	ids := make([]string, 0, len(rfcMessageIDs))
	for _, rfc := range rfcMessageIDs {
		id, err := c.FindByRFCMessageID(ctx, rfc)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if spam {
		return c.Modify(ctx, ids, []string{"SPAM"}, []string{"INBOX"})
	}
	return c.Modify(ctx, ids, []string{"INBOX"}, []string{"SPAM"})
}
