package gmail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const ScopeSettingsBasic = "https://www.googleapis.com/auth/gmail.settings.basic"

// ScopesCoverFilters deliberately requires the explicit settings grant. Unknown
// grants and full-mail grants are not assumed to include filter management.
func ScopesCoverFilters(scopes []string) bool { return hasAny(scopes, ScopeSettingsBasic) }

// Filter is a Gmail-owned rule, not a local substring rule. Gmail evaluates all
// matching filters and has no first-match ordering or update operation.
type Filter struct {
	ID       string         `json:"id,omitempty"`
	Criteria FilterCriteria `json:"criteria"`
	Action   FilterAction   `json:"action"`
}

// FilterCriteria contains provider-native Gmail matching fields.
type FilterCriteria struct {
	From           string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	Subject        string `json:"subject,omitempty"`
	Query          string `json:"query,omitempty"`
	NegatedQuery   string `json:"negatedQuery,omitempty"`
	HasAttachment  bool   `json:"hasAttachment,omitempty"`
	ExcludeChats   bool   `json:"excludeChats,omitempty"`
	Size           int64  `json:"size,omitempty"`
	SizeComparison string `json:"sizeComparison,omitempty"`
}

// FilterAction contains provider-native Gmail label and forwarding actions.
type FilterAction struct {
	AddLabelIDs    []string `json:"addLabelIds,omitempty"`
	RemoveLabelIDs []string `json:"removeLabelIds,omitempty"`
	Forward        string   `json:"forward,omitempty"`
}

// ErrFilterOutcomeUnknown means the single write attempt may have taken effect.
// Callers must reconcile against list/get, never automatically replay create.
var ErrFilterOutcomeUnknown = errors.New("gmail: filter write outcome needs verification")

func filterPath(id string) (string, error) {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "/\\?#") {
		return "", fmt.Errorf("invalid Gmail filter ID")
	}
	return "/settings/filters/" + url.PathEscape(id), nil
}

// ListFilters returns the bounded provider catalog (Gmail allows 1000 rules).
func (c *Client) ListFilters(ctx context.Context) ([]Filter, error) {
	var response struct {
		Filters []Filter `json:"filter"`
	}
	if err := c.do(ctx, http.MethodGet, "/settings/filters", nil, nil, &response); err != nil {
		return nil, err
	}
	if len(response.Filters) > 1000 {
		return nil, fmt.Errorf("Gmail filter catalog exceeds limit")
	}
	return response.Filters, nil
}

// GetFilter reads one exact provider filter identity.
func (c *Client) GetFilter(ctx context.Context, id string) (*Filter, error) {
	path, err := filterPath(id)
	if err != nil {
		return nil, err
	}
	var result Filter
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &result); err != nil {
		return nil, err
	}
	if result.ID != id {
		return nil, fmt.Errorf("Gmail returned a different filter identity")
	}
	return &result, nil
}

// CreateFilter makes one non-retried provider create attempt.
func (c *Client) CreateFilter(ctx context.Context, filter Filter) (*Filter, error) {
	if filter.ID != "" {
		return nil, fmt.Errorf("new Gmail filter must not have an ID")
	}
	if filter.Criteria == (FilterCriteria{}) {
		return nil, fmt.Errorf("Gmail filter criteria required")
	}
	if len(filter.Action.AddLabelIDs)+len(filter.Action.RemoveLabelIDs) == 0 && filter.Action.Forward == "" {
		return nil, fmt.Errorf("Gmail filter action required")
	}
	var result Filter
	// Create has no idempotency key. Disable transport/5xx retries, including
	// malformed success replies, because a duplicate could execute twice.
	if err := c.request(ctx, http.MethodPost, "/settings/filters", nil, filter, &result, false, maxResponseBytes); err != nil {
		return nil, filterWriteError(err)
	}
	if result.ID == "" {
		return nil, ErrFilterOutcomeUnknown
	}
	return &result, nil
}

// DeleteFilter submits one non-retried provider deletion.
func (c *Client) DeleteFilter(ctx context.Context, id string) error {
	path, err := filterPath(id)
	if err != nil {
		return err
	}
	if err := c.request(ctx, http.MethodDelete, path, nil, nil, nil, false, maxResponseBytes); err != nil {
		return filterWriteError(err)
	}
	return nil
}
func filterWriteError(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		return err
	}
	return fmt.Errorf("%w: %w", ErrFilterOutcomeUnknown, err)
}
