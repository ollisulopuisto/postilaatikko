package alwayson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/oauth2"
)

type helperGmailProvider struct {
	runtime                   *SyncRuntime
	accountID, email, binding string
}

func (p helperGmailProvider) AccountID() string    { return p.accountID }
func (p helperGmailProvider) OwnerBinding() string { return p.binding }
func helperGmailBinding(email, project string) string {
	data, _ := json.Marshal([]string{strings.ToLower(strings.TrimSpace(email)), project})
	return string(data)
}
func (r *SyncRuntime) gmailPublicationOwner(accountID string) (helperGmailProvider, bool, error) {
	p := helperGmailProvider{runtime: r, accountID: accountID}
	if r == nil || r.contacts == nil || r.contacts.creds == nil || r.accounts == nil || r.rules == nil {
		return p, false, fmt.Errorf("helper account owner unavailable")
	}
	acc, err := r.accounts.Get(accountID)
	if err != nil {
		return p, false, err
	}
	if acc == nil || !acc.Enabled || acc.AuthType != account.AuthOAuth2 || acc.IMAPHost != "imap.gmail.com" || strings.TrimSpace(acc.Email) == "" {
		return p, false, fmt.Errorf("enabled helper-owned Google account required")
	}
	tokens, err := r.contacts.creds.GetOAuthTokens(accountID)
	if err != nil {
		return p, false, err
	}
	defer tokens.Clear()
	if tokens.Provider != "google" || tokens.RefreshToken == "" {
		return p, false, fmt.Errorf("existing helper Google Mail grant required")
	}
	cfg, err := oauth2.GetProvider("google")
	if err != nil {
		return p, false, err
	}
	if cfg.ClientID == "" {
		return p, false, fmt.Errorf("helper Google Mail OAuth project missing")
	}
	p.email = acc.Email
	p.binding = helperGmailBinding(acc.Email, cfg.ClientID)
	return p, gmail.ScopesCoverFilters(tokens.Scopes), nil
}
func (p helperGmailProvider) client(ctx context.Context) (*gmail.Client, error) {
	current, granted, err := p.runtime.gmailPublicationOwner(p.accountID)
	if err != nil {
		return nil, err
	}
	if !granted || current.binding != p.binding {
		return nil, fmt.Errorf("helper Gmail grant or owner changed")
	}
	if p.runtime.gmailFilterTokens == nil {
		return nil, fmt.Errorf("helper settings token source unavailable")
	}
	token, err := p.runtime.gmailFilterTokens(ctx, p.accountID, false)
	if err != nil {
		return nil, err
	}
	client := gmail.NewClient(func(_ context.Context, force bool) (string, error) {
		if force {
			return "", fmt.Errorf("verified helper token expired; no write retry")
		}
		return token, nil
	})
	actual, err := client.ProfileEmail(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(p.email)) {
		return nil, fmt.Errorf("authenticated Gmail identity differs from helper review")
	}
	return client, nil
}
func (p helperGmailProvider) ListFilters(ctx context.Context) ([]gmail.Filter, error) {
	c, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.ListFilters(ctx)
}
func (p helperGmailProvider) CreateFilter(ctx context.Context, f gmail.Filter) (*gmail.Filter, error) {
	c, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.CreateFilter(ctx, f)
}
func (p helperGmailProvider) GetFilter(ctx context.Context, id string) (*gmail.Filter, error) {
	c, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetFilter(ctx, id)
}
func helperNativeDraft(f gmail.Filter) (gmail.Filter, error) {
	q := strings.TrimSpace(f.Criteria.Query)
	if q == "" || len(q) > 2048 || strings.ContainsFunc(q, unicode.IsControl) || f.ID != "" || f.Criteria != (gmail.FilterCriteria{Query: f.Criteria.Query}) || f.Action.Forward != "" {
		return f, fmt.Errorf("explicit Gmail query and label-only actions required")
	}
	f.Criteria.Query = q
	f.Action.AddLabelIDs = slices.Clone(f.Action.AddLabelIDs)
	f.Action.RemoveLabelIDs = slices.Clone(f.Action.RemoveLabelIDs)
	slices.Sort(f.Action.AddLabelIDs)
	slices.Sort(f.Action.RemoveLabelIDs)
	f.Action.AddLabelIDs = slices.Compact(f.Action.AddLabelIDs)
	f.Action.RemoveLabelIDs = slices.Compact(f.Action.RemoveLabelIDs)
	if len(f.Action.AddLabelIDs) == 0 {
		f.Action.AddLabelIDs = nil
	}
	if len(f.Action.RemoveLabelIDs) == 0 {
		f.Action.RemoveLabelIDs = nil
	}
	if len(f.Action.AddLabelIDs)+len(f.Action.RemoveLabelIDs) == 0 {
		return f, fmt.Errorf("choose at least one label action")
	}
	if slices.Contains(f.Action.AddLabelIDs, "TRASH") && slices.Contains(f.Action.RemoveLabelIDs, "INBOX") {
		return f, fmt.Errorf("choose archive or trash, not both")
	}
	for _, id := range f.Action.RemoveLabelIDs {
		if id != "UNREAD" && id != "INBOX" {
			return f, fmt.Errorf("unsupported removed label")
		}
	}
	return f, nil
}
func helperPublicationFingerprint(source *mailfilter.Rule, owner string, f gmail.Filter) string {
	copy := *source
	copy.CreatedAt = copy.CreatedAt.UTC()
	copy.UpdatedAt = copy.UpdatedAt.UTC()
	data, err := json.Marshal(struct {
		Source mailfilter.Rule
		Owner  string
		Filter gmail.Filter
	}{copy, owner, f})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func helperAllowedLabels(f gmail.Filter, labels []gmail.Label) ([]string, error) {
	allowed := []string{}
	user := map[string]bool{}
	for _, label := range labels {
		allowed = append(allowed, label.ID)
		if label.Type == "user" {
			user[label.ID] = true
		}
	}
	for _, id := range f.Action.AddLabelIDs {
		if id != "TRASH" && !user[id] {
			return nil, fmt.Errorf("choose user labels from the helper-owned account")
		}
	}
	for _, id := range append(slices.Clone(f.Action.AddLabelIDs), f.Action.RemoveLabelIDs...) {
		if !slices.Contains(allowed, id) {
			return nil, fmt.Errorf("label no longer exists in helper-owned account")
		}
	}
	return allowed, nil
}

// GmailPublication serves native-filter review on the account-owning helper.
// It requires an already granted settings scope. Consent exchange is separate;
// this method never reads desktop credentials or starts consent implicitly.
func (r *SyncRuntime) GmailPublication(ctx context.Context, req v1.GmailPublicationRequest) (v1.GmailPublicationReply, error) {
	result := v1.GmailPublicationReply{AccountID: req.AccountID}
	p, granted, err := r.gmailPublicationOwner(req.AccountID)
	if err != nil {
		return result, err
	}
	result.Email = p.email
	result.ConsentRequired = !granted
	if req.Operation == "status" || req.Operation == "publish" || req.Operation == "reconcile" || req.Operation == "cancel" {
		plan, err := r.rules.GetGmailPublication(req.ID)
		if err != nil {
			return result, err
		}
		if plan.AccountID != req.AccountID {
			return result, fmt.Errorf("publication account mismatch")
		}
		result.Publication = plan
		result.Fingerprint = helperPublicationFingerprint(&plan.Source, plan.OwnerBinding, plan.Filter)
		if req.Operation == "status" {
			return result, nil
		}
		if req.Operation == "cancel" {
			if err := r.rules.CancelGmailPublication(plan.ID); err != nil {
				return result, err
			}
			result.Publication = nil
			result.Fingerprint = ""
			return result, nil
		}
		if !granted || plan.OwnerBinding == "" || plan.OwnerBinding != p.binding {
			return result, fmt.Errorf("publication owner or explicit settings grant changed")
		}
		switch req.Operation {
		case "publish":
			if req.Fingerprint == "" || req.Fingerprint != result.Fingerprint {
				return result, fmt.Errorf("frozen publication review mismatch")
			}
			result.Publication, err = r.rules.PublishGmailFilter(ctx, plan.ID, p)
		case "reconcile":
			result.Publication, err = r.rules.ReconcileGmailPublication(ctx, plan.ID, p)
		}
		return result, err
	}
	source, err := r.rules.Get(req.RuleID)
	if err != nil {
		return result, err
	}
	if source == nil || source.Enabled || len(source.ScopeAccountIDs()) != 1 || source.ScopeAccountIDs()[0] != req.AccountID || len(source.FolderIDs) > 0 || source.DestFolderID != "" {
		return result, fmt.Errorf("stop a helper rule scoped to this account without folder/move scope")
	}
	if !granted {
		if req.Operation == "context" {
			return result, nil
		}
		return result, fmt.Errorf("explicit helper Gmail settings consent required")
	}
	c, err := p.client(ctx)
	if err != nil {
		return result, err
	}
	labels, err := c.Labels(ctx)
	if err != nil {
		return result, err
	}
	if req.Operation == "context" {
		for _, label := range labels {
			if label.Type == "user" {
				result.Labels = append(result.Labels, label)
			}
		}
		return result, nil
	}
	f, err := helperNativeDraft(req.Filter)
	if err != nil {
		return result, err
	}
	allowed, err := helperAllowedLabels(f, labels)
	if err != nil {
		return result, err
	}
	result.Fingerprint = helperPublicationFingerprint(source, p.binding, f)
	if result.Fingerprint == "" {
		return result, fmt.Errorf("source cannot be fingerprinted")
	}
	switch req.Operation {
	case "preview":
		page, err := c.Search(ctx, gmail.SearchRequest{Query: f.Criteria.Query, Limit: 51, IncludeSpamTrash: true})
		if err != nil {
			return result, err
		}
		result.Found = len(page.MessageIDs)
		result.Truncated = result.Found > 50 || page.NextPageToken != ""
		if result.Found > 50 {
			result.Found = 50
		}
	case "prepare":
		if req.Fingerprint != result.Fingerprint {
			return result, fmt.Errorf("helper review changed; preview again")
		}
		catalog, err := p.ListFilters(ctx)
		if err != nil {
			return result, err
		}
		plan, err := r.rules.PrepareGmailPublication(source.ID, req.AccountID, f, catalog, allowed, p.binding)
		if err != nil {
			return result, err
		}
		if helperPublicationFingerprint(&plan.Source, plan.OwnerBinding, plan.Filter) != result.Fingerprint {
			if cancelErr := r.rules.CancelGmailPublication(plan.ID); cancelErr != nil {
				return result, fmt.Errorf("source changed; reservation needs recovery")
			}
			return result, fmt.Errorf("source changed; preview again")
		}
		result.Publication = plan
	default:
		return result, fmt.Errorf("unknown helper Gmail operation")
	}
	return result, nil
}
