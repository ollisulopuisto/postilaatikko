package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailfilter"
)

// NativeGmailLabel is a non-secret, account-qualified provider label choice.
type NativeGmailLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NativeGmailContext describes one eligible stopped source and its account.
type NativeGmailContext struct {
	AccountID           string             `json:"accountId"`
	Email               string             `json:"email"`
	ConsentRequired     bool               `json:"consentRequired"`
	Labels              []NativeGmailLabel `json:"labels"`
	ForwardingAddresses []string           `json:"forwardingAddresses"`
}

// NativeGmailDraft is explicitly written in Gmail search syntax, never an
// automatic conversion of the source's literal substring criteria.
type NativeGmailDraft struct {
	RuleID   string   `json:"ruleId"`
	Query    string   `json:"query"`
	MarkRead bool     `json:"markRead"`
	Archive  bool     `json:"archive"`
	Trash    bool     `json:"trash"`
	LabelIDs []string `json:"labelIds"`
	Forward  string   `json:"forward"`
}

// NativeGmailPreview is a bounded historical sample, not an exact count or a
// guarantee about future incoming-mail matching. No old mail is changed.
type NativeGmailPreview struct {
	AccountEmail string `json:"accountEmail"`
	Found        int    `json:"found"`
	Truncated    bool   `json:"truncated"`
	Fingerprint  string `json:"fingerprint"`
}

// NativeGmailPublicationStatus keeps the reviewed attempt recoverable in UI.
type NativeGmailPublicationStatus struct {
	AccountEmail   string   `json:"accountEmail"`
	RuleID         string   `json:"ruleId"`
	AccountID      string   `json:"accountId"`
	Query          string   `json:"query"`
	AddLabelIDs    []string `json:"addLabelIds"`
	RemoveLabelIDs []string `json:"removeLabelIds"`
	Forward        string   `json:"forward"`

	ID          string `json:"id"`
	State       string `json:"state"`
	ProviderID  string `json:"providerId"`
	Fingerprint string `json:"fingerprint"`
}

func (a *App) nativeGmailSource(ruleID string) (*mailfilter.Rule, string, error) {
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return nil, "", err
	}
	rule, err := a.filterStore.Get(ruleID)
	if err != nil {
		return nil, "", err
	}
	if rule == nil || rule.Enabled || len(rule.ScopeAccountIDs()) != 1 || len(rule.FolderIDs) > 0 || rule.DestFolderID != "" {
		return nil, "", fmt.Errorf("disable a single-account rule without folder or move scope before native Gmail review")
	}
	return rule, rule.ScopeAccountIDs()[0], nil
}

// GetNativeGmailContext loads account/label choices without starting consent.
func (a *App) GetNativeGmailContext(ruleID string) (NativeGmailContext, error) {
	result := NativeGmailContext{}
	_, accountID, err := a.nativeGmailSource(ruleID)
	if err != nil {
		return result, err
	}
	email, tokens, _, err := a.prepareGmailFilterConsent(accountID)
	if err != nil {
		return result, err
	}
	defer tokens.Clear()
	result.AccountID, result.Email, result.ConsentRequired = accountID, email, !gmail.ScopesCoverFilters(tokens.Scopes)
	result.Labels = []NativeGmailLabel{}
	if result.ConsentRequired {
		return result, nil
	}
	provider, err := a.gmailFilterProvider(accountID)
	if err != nil {
		return result, err
	}
	client, err := provider.snapshotClient(a.helperCommandContext())
	if err != nil {
		return result, err
	}
	labels, err := client.Labels(a.helperCommandContext())
	if err != nil {
		return result, err
	}
	for _, label := range labels {
		if label.Type == "user" {
			result.Labels = append(result.Labels, NativeGmailLabel{ID: label.ID, Name: label.Name})
		}
	}
	addresses, err := client.ListForwardingAddresses(a.helperCommandContext())
	if err != nil {
		return result, err
	}
	for _, entry := range addresses {
		if entry.VerificationStatus == "accepted" && gmail.ValidForwardingAddress(entry.Email) {
			result.ForwardingAddresses = append(result.ForwardingAddresses, entry.Email)
		}
	}
	return result, nil
}
func nativeGmailPayload(draft NativeGmailDraft) (gmail.Filter, error) {
	query := strings.TrimSpace(draft.Query)
	if query == "" || len(query) > 2048 || strings.ContainsFunc(query, unicode.IsControl) {
		return gmail.Filter{}, fmt.Errorf("enter a Gmail query of at most 2048 bytes without control characters")
	}
	if draft.Archive && draft.Trash {
		return gmail.Filter{}, fmt.Errorf("choose Archive or Trash, not both")
	}
	if draft.Forward != "" && !gmail.ValidForwardingAddress(draft.Forward) {
		return gmail.Filter{}, fmt.Errorf("choose an existing Gmail forwarding address")
	}
	f := gmail.Filter{Criteria: gmail.FilterCriteria{Query: query}, Action: gmail.FilterAction{Forward: draft.Forward}}
	for _, id := range draft.LabelIDs {
		if id == "" || len(id) > 256 || strings.ContainsFunc(id, unicode.IsControl) {
			return gmail.Filter{}, fmt.Errorf("choose valid provider user-label IDs")
		}
		f.Action.AddLabelIDs = append(f.Action.AddLabelIDs, id)
	}
	slices.Sort(f.Action.AddLabelIDs)
	f.Action.AddLabelIDs = slices.Compact(f.Action.AddLabelIDs)
	if draft.MarkRead {
		f.Action.RemoveLabelIDs = append(f.Action.RemoveLabelIDs, "UNREAD")
	}
	if draft.Archive {
		f.Action.RemoveLabelIDs = append(f.Action.RemoveLabelIDs, "INBOX")
	}
	if draft.Trash {
		f.Action.AddLabelIDs = append(f.Action.AddLabelIDs, "TRASH")
	}
	slices.Sort(f.Action.AddLabelIDs)
	slices.Sort(f.Action.RemoveLabelIDs)
	if len(f.Action.AddLabelIDs)+len(f.Action.RemoveLabelIDs) == 0 && f.Action.Forward == "" {
		return gmail.Filter{}, fmt.Errorf("choose at least one label or forwarding action")
	}
	return f, nil
}
func validateNativeUserLabels(ctx context.Context, provider accountFilterProvider, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	labels, err := provider.client.Labels(ctx)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, label := range labels {
		if label.Type == "user" {
			allowed[label.ID] = true
		}
	}
	for _, id := range ids {
		if !allowed[id] {
			return fmt.Errorf("chosen label is not a user label of this account")
		}
	}
	return nil
}
func nativeGmailFingerprint(rule *mailfilter.Rule, accountID string, f gmail.Filter, binding ...string) string {
	owner := ""
	if len(binding) > 0 {
		owner = binding[0]
	}
	source := *rule
	source.CreatedAt, source.UpdatedAt = source.CreatedAt.UTC(), source.UpdatedAt.UTC()
	data, err := json.Marshal(struct {
		Source       mailfilter.Rule
		OwnerBinding string
		AccountID    string
		Filter       gmail.Filter
	}{source, owner, accountID, f})
	if err != nil {
		return "" // Invalid source timestamps cannot acquire a review fingerprint.
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func previewNativeGmail(ctx context.Context, client *gmail.Client, f gmail.Filter) (int, bool, error) {
	page, err := client.Search(ctx, gmail.SearchRequest{Query: f.Criteria.Query, Limit: 51, IncludeSpamTrash: true})
	if err != nil {
		return 0, false, err
	}
	count := len(page.MessageIDs)
	truncated := count > 50 || page.NextPageToken != ""
	if count > 50 {
		count = 50
	}
	return count, truncated, nil
}

// PreviewNativeGmailFilter reads at most one bounded provider results page.
func (a *App) PreviewNativeGmailFilter(draft NativeGmailDraft) (NativeGmailPreview, error) {
	result := NativeGmailPreview{}
	rule, accountID, err := a.nativeGmailSource(draft.RuleID)
	if err != nil {
		return result, err
	}
	f, err := nativeGmailPayload(draft)
	if err != nil {
		return result, err
	}
	provider, err := a.gmailFilterProvider(accountID)
	if err != nil {
		return result, err
	}
	if err := validateNativeUserLabels(a.helperCommandContext(), provider, draft.LabelIDs); err != nil {
		return result, err
	}
	if f.Action.Forward != "" {
		client, err := provider.snapshotClient(a.helperCommandContext())
		if err != nil {
			return result, err
		}
		addresses, err := client.ListForwardingAddresses(a.helperCommandContext())
		if err != nil {
			return result, err
		}
		if err := gmail.ValidateForwardingDestination(f.Action.Forward, addresses); err != nil {
			return result, err
		}
	}
	result.Found, result.Truncated, err = previewNativeGmail(a.helperCommandContext(), provider.client, f)
	if err != nil {
		return result, err
	}
	result.AccountEmail = provider.email
	result.Fingerprint = nativeGmailFingerprint(rule, accountID, f, provider.OwnerBinding())
	if result.Fingerprint == "" {
		return result, fmt.Errorf("source could not be fingerprinted")
	}
	return result, nil
}
func publicationStatus(p *mailfilter.GmailPublication) NativeGmailPublicationStatus {
	var owner []string
	_ = json.Unmarshal([]byte(p.OwnerBinding), &owner)
	email := ""
	if len(owner) == 2 {
		email = owner[0]
	}
	return NativeGmailPublicationStatus{Forward: p.Filter.Action.Forward, AccountEmail: email, RuleID: p.RuleID, AccountID: p.AccountID, Query: p.Filter.Criteria.Query, AddLabelIDs: p.Filter.Action.AddLabelIDs, RemoveLabelIDs: p.Filter.Action.RemoveLabelIDs, ID: p.ID, State: p.State, ProviderID: p.ProviderID, Fingerprint: nativeGmailFingerprint(&p.Source, p.AccountID, p.Filter, p.OwnerBinding)}
}

// PrepareNativeGmailFilter freezes the exact previewed payload for final review.
// It does not create a Gmail filter or apply anything to existing mail.
func (a *App) PrepareNativeGmailFilter(draft NativeGmailDraft, fingerprint string) (NativeGmailPublicationStatus, error) {
	result := NativeGmailPublicationStatus{}
	rule, accountID, err := a.nativeGmailSource(draft.RuleID)
	if err != nil {
		return result, err
	}
	f, err := nativeGmailPayload(draft)
	if err != nil {
		return result, err
	}
	provider, err := a.gmailFilterProvider(accountID)
	if err != nil {
		return result, err
	}
	if fingerprint == "" || fingerprint != nativeGmailFingerprint(rule, accountID, f, provider.OwnerBinding()) {
		return result, fmt.Errorf("review or account changed; preview again")
	}
	if err := validateNativeUserLabels(a.helperCommandContext(), provider, draft.LabelIDs); err != nil {
		return result, err
	}
	plan, err := a.prepareLocalGmailPublication(a.helperCommandContext(), draft.RuleID, accountID, f)
	if err != nil {
		return result, err
	}
	status := publicationStatus(plan)
	if status.Fingerprint != fingerprint {
		if cancelErr := a.filterStore.CancelGmailPublication(plan.ID); cancelErr != nil {
			return status, fmt.Errorf("source changed and review remains reserved: %w", cancelErr)
		}
		return result, fmt.Errorf("source changed during review; preview again")
	}
	return status, nil
}

// PublishNativeGmailFilter makes the reviewed plan's single provider attempt.
func (a *App) PublishNativeGmailFilter(id, fingerprint string) (NativeGmailPublicationStatus, error) {
	result := NativeGmailPublicationStatus{}
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return result, err
	}
	plan, err := a.filterStore.GetGmailPublication(id)
	if err != nil {
		return result, err
	}
	result = publicationStatus(plan)
	if fingerprint == "" || fingerprint != result.Fingerprint {
		return result, fmt.Errorf("publication review mismatch")
	}
	updated, err := a.publishLocalGmailPublication(a.helperCommandContext(), id)
	if updated != nil {
		return publicationStatus(updated), err
	}
	latest, readErr := a.filterStore.GetGmailPublication(id)
	if readErr == nil {
		result = publicationStatus(latest)
	}
	return result, err
}

// ReconcileNativeGmailFilter only reads the provider, never retries create.
func (a *App) ReconcileNativeGmailFilter(id string) (NativeGmailPublicationStatus, error) {
	result := NativeGmailPublicationStatus{}
	plan, err := a.reconcileLocalGmailPublication(a.helperCommandContext(), id)
	if plan != nil {
		result = publicationStatus(plan)
	}
	return result, err
}

// CancelNativeGmailFilter releases only an unattempted local review plan.
func (a *App) CancelNativeGmailFilter(id string) error {
	if err := a.requireStandaloneFilterOwner(); err != nil {
		return err
	}
	return a.filterStore.CancelGmailPublication(id)
}

// GetNativeGmailPublication recovers a rule's durable review/attempt after close.
func (a *App) GetNativeGmailPublication(ruleID string) (NativeGmailPublicationStatus, error) {
	empty := NativeGmailPublicationStatus{}
	_, accountID, err := a.nativeGmailSource(ruleID)
	if err != nil {
		return empty, err
	}
	ids, err := a.filterStore.ListGmailPublicationIDs(accountID)
	if err != nil {
		return empty, err
	}
	for _, id := range ids {
		p, err := a.filterStore.GetGmailPublication(id)
		if err != nil {
			return empty, err
		}
		if p.RuleID == ruleID {
			return publicationStatus(p), nil
		}
	}
	return empty, nil
}
