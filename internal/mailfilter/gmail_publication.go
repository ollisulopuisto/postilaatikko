package mailfilter

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/google/uuid"
	"github.com/hkdb/aerion/internal/gmail"
)

// NativeFilterProvider is the provider-owned catalog, never a local rule store.
type NativeFilterProvider interface {
	AccountID() string
	ListFilters(context.Context) ([]gmail.Filter, error)
	CreateFilter(context.Context, gmail.Filter) (*gmail.Filter, error)
	GetFilter(context.Context, string) (*gmail.Filter, error)
}

// GmailPublication is durable recovery state. Attempted and unknown records
// must be reconciled, never replayed, even after a process restart.
type GmailPublication struct {
	ID          string       `json:"id"`
	RuleID      string       `json:"ruleId"`
	AccountID   string       `json:"accountId"`
	State       string       `json:"state"`
	ProviderID  string       `json:"providerId"`
	Source      Rule         `json:"source"`
	Filter      gmail.Filter `json:"filter"`
	BaselineIDs []string     `json:"baselineIds"`
}

func canonicalNativeFilter(f gmail.Filter) gmail.Filter {
	f.ID = ""
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
	return f
}
func sameNativeFilter(a, b gmail.Filter) bool {
	return reflect.DeepEqual(canonicalNativeFilter(a), canonicalNativeFilter(b))
}

// PrepareGmailPublication records the exact provider-native draft reviewed by
// the caller. It does not translate local substring criteria, request consent,
// stop a running executor, or contact Gmail. The source must already be stopped.
// allowedLabelIDs must be the selected account's freshly fetched label catalog.
func (s *Store) PrepareGmailPublication(ruleID, accountID string, draft gmail.Filter, baseline []gmail.Filter, allowedLabelIDs []string) (*GmailPublication, error) {
	if draft.ID != "" || draft.Criteria == (gmail.FilterCriteria{}) || draft.Action.Forward != "" {
		return nil, fmt.Errorf("new provider-native criteria and label-only actions required")
	}
	if len(baseline) > 1000 {
		return nil, fmt.Errorf("provider catalog exceeds limit")
	}
	draft = canonicalNativeFilter(draft)
	encoded, err := json.Marshal(draft)
	if err != nil || len(encoded) > 16384 {
		return nil, fmt.Errorf("provider draft exceeds size limit")
	}

	if len(draft.Action.AddLabelIDs)+len(draft.Action.RemoveLabelIDs) == 0 {
		return nil, fmt.Errorf("label action required")
	}
	for _, id := range append(slices.Clone(draft.Action.AddLabelIDs), draft.Action.RemoveLabelIDs...) {
		if id == "" || !slices.Contains(allowedLabelIDs, id) {
			return nil, fmt.Errorf("action label not in selected account catalog")
		}
		if slices.Contains(draft.Action.AddLabelIDs, id) && slices.Contains(draft.Action.RemoveLabelIDs, id) {
			return nil, fmt.Errorf("conflicting label action")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	source, err := scanRule(tx.QueryRow(`SELECT `+ruleColumns+` FROM mail_filter_rules WHERE id=?`, ruleID))
	if err != nil {
		return nil, err
	}
	if source == nil || source.Enabled || len(source.ScopeAccountIDs()) != 1 || source.ScopeAccountIDs()[0] != accountID || len(source.FolderIDs) != 0 || source.DestFolderID != "" {
		return nil, fmt.Errorf("stopped single-account rule without folder/move scope required")
	}
	p := &GmailPublication{ID: uuid.NewString(), RuleID: ruleID, AccountID: accountID, State: "prepared", Source: *source, Filter: draft}
	for _, f := range baseline {
		if f.ID == "" {
			return nil, fmt.Errorf("provider catalog has missing identity")
		}
		p.BaselineIDs = append(p.BaselineIDs, f.ID)
	}
	src, err := json.Marshal(p.Source)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(p.Filter)
	if err != nil {
		return nil, err
	}
	ids, err := json.Marshal(p.BaselineIDs)
	if err != nil {
		return nil, err
	}
	// One statement checks stopped ownership and outstanding jobs while taking
	// the durable guard that Store.Update observes when enabling the rule.
	result, err := tx.Exec(`INSERT INTO gmail_filter_publications(id,rule_id,account_id,state,source_json,filter_json,baseline_ids)
 SELECT ?,?,?,'prepared',?,?,? FROM mail_filter_rules r WHERE r.id=? AND r.enabled=0
 AND NOT EXISTS(SELECT 1 FROM automatic_filter_jobs j WHERE j.rule_id=r.id)`, p.ID, ruleID, accountID, string(src), string(payload), string(ids), ruleID)
	if err != nil {
		return nil, fmt.Errorf("prepare publication: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, fmt.Errorf("source running or filter jobs require reconciliation")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

// GetGmailPublication loads durable recovery state without provider effects.
func (s *Store) GetGmailPublication(id string) (*GmailPublication, error) {
	p := &GmailPublication{}
	var src, payload, ids string
	err := s.db.QueryRow(`SELECT id,rule_id,account_id,state,provider_id,source_json,filter_json,baseline_ids FROM gmail_filter_publications WHERE id=?`, id).Scan(&p.ID, &p.RuleID, &p.AccountID, &p.State, &p.ProviderID, &src, &payload, &ids)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(src), &p.Source); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(payload), &p.Filter); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(ids), &p.BaselineIDs); err != nil {
		return nil, err
	}
	return p, nil
}

// PublishGmailFilter performs at most one create attempt for this record.
// The account-owning caller supplies a client bound to p.AccountID and its
// explicit settings grant. This core is not exposed to the UI on its own.
func (s *Store) PublishGmailFilter(ctx context.Context, id string, provider NativeFilterProvider) (*GmailPublication, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider required")
	}
	p, err := s.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	if provider.AccountID() != p.AccountID {
		return nil, fmt.Errorf("provider account mismatch")
	}
	if p.State != "prepared" {
		return nil, fmt.Errorf("publication already attempted; reconcile instead")
	}
	source, err := s.Get(p.RuleID)
	if err != nil {
		return nil, err
	}
	if source == nil || source.Enabled || !SameRuleConfiguration(source, &p.Source) {
		return nil, fmt.Errorf("source changed; publication blocked")
	}
	current, err := provider.ListFilters(ctx)
	if err != nil {
		return nil, err
	}
	// The reviewed provider catalog must still have the same identities and
	// no identical draft. A changed catalog needs a new review, not a guess.
	ids := []string{}
	for _, f := range current {
		ids = append(ids, f.ID)
		if sameNativeFilter(f, p.Filter) {
			return nil, fmt.Errorf("matching Gmail filter already exists")
		}
	}
	expected := slices.Clone(p.BaselineIDs)
	slices.Sort(ids)
	slices.Sort(expected)
	if !slices.Equal(ids, expected) {
		return nil, fmt.Errorf("provider catalog changed; review again")
	}
	result, err := s.db.Exec(`UPDATE gmail_filter_publications SET state='attempted',updated_at=datetime('now') WHERE id=? AND state='prepared' AND NOT EXISTS(SELECT 1 FROM automatic_filter_jobs WHERE rule_id=?)`, id, p.RuleID)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, fmt.Errorf("publication changed or jobs outstanding")
	}
	created, writeErr := provider.CreateFilter(ctx, p.Filter)
	providerID := ""
	if created != nil {
		providerID = created.ID
	}
	// Unknown is durable before read-back. Any failure here leaves attempted,
	// which is equally non-replayable and safe for subsequent reconciliation.
	if _, err = s.db.Exec(`UPDATE gmail_filter_publications SET state='unknown',provider_id=?,updated_at=datetime('now') WHERE id=?`, providerID, id); err != nil {
		return nil, err
	}
	if writeErr != nil {
		return nil, fmt.Errorf("publication outcome requires reconciliation: %w", writeErr)
	}
	return s.ReconcileGmailPublication(ctx, id, provider)
}

// ReconcileGmailPublication reads only. No candidate, multiple candidates or a
// changed known provider ID remain uncertain; none permits another create.
func (s *Store) ReconcileGmailPublication(ctx context.Context, id string, provider NativeFilterProvider) (*GmailPublication, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider required")
	}
	p, err := s.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	if provider.AccountID() != p.AccountID {
		return nil, fmt.Errorf("provider account mismatch")
	}
	if p.State == "prepared" {
		return nil, fmt.Errorf("publication has not been attempted")
	}
	var candidates []gmail.Filter
	if p.ProviderID != "" {
		f, err := provider.GetFilter(ctx, p.ProviderID)
		if err != nil {
			return nil, err
		}
		if f != nil {
			candidates = append(candidates, *f)
		}
	}
	if p.ProviderID == "" {
		catalog, err := provider.ListFilters(ctx)
		if err != nil {
			return nil, err
		}
		for _, f := range catalog {
			if !slices.Contains(p.BaselineIDs, f.ID) && sameNativeFilter(f, p.Filter) {
				candidates = append(candidates, f)
			}
		}
	}
	if len(candidates) != 1 || candidates[0].ID == "" || !sameNativeFilter(candidates[0], p.Filter) {
		return nil, fmt.Errorf("Gmail publication not uniquely verified; do not retry create")
	}
	if _, err = s.db.Exec(`UPDATE gmail_filter_publications SET state='published',provider_id=?,updated_at=datetime('now') WHERE id=?`, candidates[0].ID, id); err != nil {
		return nil, err
	}
	return s.GetGmailPublication(id)
}

// CancelGmailPublication removes only an unattempted plan, restoring local
// editing freedom. Once claimed, provider reconciliation is mandatory.
func (s *Store) CancelGmailPublication(id string) error {
	result, err := s.db.Exec(`DELETE FROM gmail_filter_publications WHERE id=? AND state='prepared'`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("publication already attempted or missing; reconcile instead")
	}
	return nil
}

// ListGmailPublicationIDs recovers account-scoped records after a restart.
func (s *Store) ListGmailPublicationIDs(accountID string) ([]string, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account required")
	}
	rows, err := s.db.Query(`SELECT id FROM gmail_filter_publications WHERE account_id=? ORDER BY updated_at,id LIMIT 1001`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 1000 {
		return nil, fmt.Errorf("publication recovery catalog exceeds limit")
	}
	return ids, nil
}
