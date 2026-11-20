package mailfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
)

// Serialize live publish/recovery operations across Store instances in this
// process. SQL state/identity CAS still rejects stale cross-process updates.
var gmailPublicationMu sync.Mutex

// ErrGmailDuplicateFilter means a new draft would duplicate an existing provider rule.
var ErrGmailDuplicateFilter = errors.New("matching Gmail filter already exists; cancel this review and inspect Gmail settings")

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
	OwnerBinding string       `json:"ownerBinding,omitempty"`
	ID           string       `json:"id"`
	RuleID       string       `json:"ruleId"`
	AccountID    string       `json:"accountId"`
	State        string       `json:"state"`
	ProviderID   string       `json:"providerId"`
	Source       Rule         `json:"source"`
	Filter       gmail.Filter `json:"filter"`
	BaselineIDs  []string     `json:"baselineIds"`
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
// An optional non-secret owner binding is stored in the source snapshot envelope.
// Bound records require a matching provider OwnerBinding on publish/reconcile.
func (s *Store) PrepareGmailPublication(ruleID, accountID string, draft gmail.Filter, baseline []gmail.Filter, allowedLabelIDs []string, ownerBinding ...string) (*GmailPublication, error) {
	binding := ""
	if len(ownerBinding) > 1 {
		return nil, fmt.Errorf("one owner binding required")
	}
	if len(ownerBinding) == 1 {
		binding = ownerBinding[0]
		if binding == "" || len(binding) > 2048 {
			return nil, fmt.Errorf("invalid owner binding")
		}
	}
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
	p := &GmailPublication{OwnerBinding: binding, ID: uuid.NewString(), RuleID: ruleID, AccountID: accountID, State: "prepared", Source: *source, Filter: draft}
	for _, f := range baseline {
		if f.ID == "" {
			return nil, fmt.Errorf("provider catalog has missing identity")
		}
		p.BaselineIDs = append(p.BaselineIDs, f.ID)
	}
	src, err := json.Marshal(struct {
		Rule
		NativeOwnerBinding string `json:"nativeOwnerBinding,omitempty"`
	}{p.Source, p.OwnerBinding})
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
	var binding struct {
		NativeOwnerBinding string `json:"nativeOwnerBinding"`
	}
	if err = json.Unmarshal([]byte(src), &binding); err != nil {
		return nil, err
	}
	p.OwnerBinding = binding.NativeOwnerBinding
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
	gmailPublicationMu.Lock()
	defer gmailPublicationMu.Unlock()
	if provider == nil {
		return nil, fmt.Errorf("provider required")
	}
	p, err := s.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	if p.OwnerBinding != "" {
		bound, ok := provider.(interface{ OwnerBinding() string })
		if !ok || bound.OwnerBinding() != p.OwnerBinding {
			return nil, fmt.Errorf("provider owner changed; publication blocked")
		}
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
			return nil, ErrGmailDuplicateFilter
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
	// A pre-transport failure consumed no provider attempt. Restore the review
	// using a compare-and-swap, rather than freezing an unsent plan as unknown.
	if created == nil && errors.Is(writeErr, gmail.ErrFilterNotSubmitted) {
		result, err := s.db.Exec(`UPDATE gmail_filter_publications SET state='prepared',updated_at=datetime('now') WHERE id=? AND state='attempted' AND provider_id=''`, id)
		if err != nil {
			return nil, err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return nil, fmt.Errorf("unsent publication changed; reload status")
		}
		return nil, writeErr
	}
	// A typed definitive provider rejection proves no filter was created.
	// Keep the attempted plan terminal and non-replayable, but let the user
	// cancel it explicitly to release the local source for a new review.
	if writeErr != nil && created == nil && errors.Is(writeErr, gmail.ErrFilterRejected) {
		result, err := s.db.Exec(`UPDATE gmail_filter_publications SET state='rejected',updated_at=datetime('now') WHERE id=? AND state='attempted' AND provider_id=''`, id)
		if err != nil {
			return nil, err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return nil, fmt.Errorf("rejected publication changed concurrently; reload status")
		}
		return nil, fmt.Errorf("Gmail rejected this filter; cancel the rejected plan and review its query/actions: %w", writeErr)
	}
	providerID := ""
	if created != nil {
		providerID = created.ID
	}
	// Unknown is durable before read-back. Any failure here leaves attempted,
	// which is equally non-replayable and safe for subsequent reconciliation.
	result, err = s.db.Exec(`UPDATE gmail_filter_publications SET state='unknown',provider_id=?,updated_at=datetime('now') WHERE id=? AND state='attempted' AND provider_id=''`, providerID, id)
	if err != nil {
		return nil, err
	}
	if changed, e := result.RowsAffected(); e != nil {
		return nil, e
	} else if changed != 1 {
		// A nonempty create response is the authoritative identity of our one
		// write. Preserve it durably but do not silently bless a conflicting result.
		if providerID != "" {
			if _, saveErr := s.db.Exec(`UPDATE gmail_filter_publications SET state='unknown',provider_id=?,updated_at=datetime('now') WHERE id=?`, providerID, id); saveErr != nil {
				return nil, saveErr
			}
		}
		return nil, fmt.Errorf("publication resolved concurrently; inspect provider identities before continuing")
	}
	if writeErr != nil {
		return nil, fmt.Errorf("publication outcome requires reconciliation: %w", writeErr)
	}
	return s.reconcileGmailPublication(ctx, id, provider)
}

// ReconcileGmailPublication reads only. No candidate, multiple candidates or a
// changed known provider ID remain uncertain; none permits another create.
func (s *Store) ReconcileGmailPublication(ctx context.Context, id string, provider NativeFilterProvider) (*GmailPublication, error) {
	gmailPublicationMu.Lock()
	defer gmailPublicationMu.Unlock()
	return s.reconcileGmailPublication(ctx, id, provider)
}
func (s *Store) reconcileGmailPublication(ctx context.Context, id string, provider NativeFilterProvider) (*GmailPublication, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider required")
	}
	p, err := s.GetGmailPublication(id)
	if err != nil {
		return nil, err
	}
	if p.OwnerBinding != "" {
		bound, ok := provider.(interface{ OwnerBinding() string })
		if !ok || bound.OwnerBinding() != p.OwnerBinding {
			return nil, fmt.Errorf("provider owner changed; publication blocked")
		}
	}
	if provider.AccountID() != p.AccountID {
		return nil, fmt.Errorf("provider account mismatch")
	}
	if p.State == "prepared" || p.State == "rejected" {
		return nil, fmt.Errorf("publication has no provider result; cancel this plan before reviewing again")
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
	result, err := s.db.Exec(`UPDATE gmail_filter_publications SET state='published',provider_id=?,updated_at=datetime('now') WHERE id=? AND state=? AND provider_id=?`, candidates[0].ID, id, p.State, p.ProviderID)
	if err != nil {
		return nil, err
	}
	if changed, e := result.RowsAffected(); e != nil {
		return nil, e
	} else if changed != 1 {
		return nil, fmt.Errorf("publication changed during recovery; reload its known provider identity")
	}
	return s.GetGmailPublication(id)
}

// CancelGmailPublication releases an unattempted or definitively rejected plan.
// Ambiguous attempts and successful publications require provider reconciliation.
func (s *Store) CancelGmailPublication(id string) error {
	result, err := s.db.Exec(`DELETE FROM gmail_filter_publications WHERE id=? AND state IN ('prepared','rejected')`, id)
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

// HasGmailPublication reports whether a rule's local execution is guarded.
func (s *Store) HasGmailPublication(ruleID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM gmail_filter_publications WHERE rule_id=?`, ruleID).Scan(&count)
	return count > 0, err
}
