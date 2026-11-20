package mailfilter

import (
	"context"
	"errors"
	"testing"

	"github.com/hkdb/aerion/internal/gmail"
)

type fakeNativeProvider struct {
	ownerBinding string
	account      string
	filters      []gmail.Filter
	writes       int
	uncertain    bool
	drop         bool
	readErr      bool
}

func (p *fakeNativeProvider) OwnerBinding() string { return p.ownerBinding }
func (p *fakeNativeProvider) AccountID() string    { return p.account }
func (p *fakeNativeProvider) ListFilters(context.Context) ([]gmail.Filter, error) {
	if p.readErr {
		return nil, errors.New("offline")
	}
	return p.filters, nil
}
func (p *fakeNativeProvider) CreateFilter(_ context.Context, f gmail.Filter) (*gmail.Filter, error) {
	p.writes++
	f.ID = "created"
	if !p.drop {
		p.filters = append(p.filters, f)
	}
	if p.uncertain {
		return nil, errors.New("connection reset after submit")
	}
	return &f, nil
}
func (p *fakeNativeProvider) GetFilter(_ context.Context, id string) (*gmail.Filter, error) {
	if p.readErr {
		return nil, errors.New("offline")
	}
	for _, f := range p.filters {
		if f.ID == id {
			return &f, nil
		}
	}
	return nil, errors.New("not found")
}
func publicationFixture(t *testing.T) (*Store, *Rule, gmail.Filter, *fakeNativeProvider) {
	db := setupTestDB(t)
	t.Cleanup(func() { db.Close() })
	s := NewStore(db)
	r := &Rule{ID: "source", Name: "Rule", Enabled: false, AccountIDs: []string{"account1"}, FromContains: []string{"sender"}, ActionMarkRead: true}
	if err := s.Create(r); err != nil {
		t.Fatal(err)
	}
	f := gmail.Filter{Criteria: gmail.FilterCriteria{From: "sender@example.test"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	return s, r, f, &fakeNativeProvider{account: "account1"}
}
func preparePublication(t *testing.T, s *Store, r *Rule, f gmail.Filter, p *fakeNativeProvider) *GmailPublication {
	t.Helper()
	plan, err := s.PrepareGmailPublication(r.ID, "account1", f, p.filters, []string{"UNREAD", "INBOX", "label-one"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
func TestGmailPublicationSingleCreateAndRecovery(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "uncertain"}[unknown], func(t *testing.T) {
			s, r, f, p := publicationFixture(t)
			p.uncertain = unknown
			plan := preparePublication(t, s, r, f, p)
			result, err := s.PublishGmailFilter(context.Background(), plan.ID, p)
			if unknown && err == nil {
				t.Fatal("uncertain write accepted before reconciliation")
			}
			if !unknown && (err != nil || result.State != "published") {
				t.Fatal(err)
			}
			// Recreate the store to discard every in-memory assumption.
			restarted := NewStore(s.db)
			result, err = restarted.ReconcileGmailPublication(context.Background(), plan.ID, p)
			if err != nil || result.State != "published" || result.ProviderID != "created" {
				t.Fatal(result, err)
			}
			if _, err = restarted.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
				t.Fatal("write replayed")
			}
			if p.writes != 1 {
				t.Fatalf("writes=%d", p.writes)
			}
			r.Enabled = true
			if err = s.Update(r); err == nil {
				t.Fatal("dual executor allowed")
			}
		})
	}
}
func TestGmailPublicationUnknownNeverReplays(t *testing.T) {
	for _, mode := range []string{"none", "multiple", "known changed", "offline"} {
		t.Run(mode, func(t *testing.T) {
			s, r, f, p := publicationFixture(t)
			p.uncertain = true
			p.drop = true
			plan := preparePublication(t, s, r, f, p)
			if _, err := s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
				t.Fatal("unknown accepted")
			}
			switch mode {
			case "multiple":
				a, b := f, f
				a.ID = "a"
				b.ID = "b"
				p.filters = []gmail.Filter{a, b}
			case "known changed":
				other := f
				other.ID = "changed"
				other.Criteria.From = "other"
				p.filters = []gmail.Filter{other}
				if _, err := s.db.Exec(`UPDATE gmail_filter_publications SET provider_id='changed' WHERE id=?`, plan.ID); err != nil {
					t.Fatal(err)
				}
			case "offline":
				p.readErr = true
			}
			if _, err := s.ReconcileGmailPublication(context.Background(), plan.ID, p); err == nil {
				t.Fatal("nonunique/unread state accepted")
			}
			if _, err := s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
				t.Fatal("replayed")
			}
			if p.writes != 1 {
				t.Fatal("create repeated")
			}
		})
	}
}
func TestGmailPublicationPreflightGuards(t *testing.T) {
	for _, mode := range []string{"wrong account", "source changed", "provider changed", "already exists", "pending job", "processing job", "unknown job"} {
		t.Run(mode, func(t *testing.T) {
			s, r, f, p := publicationFixture(t)
			plan := preparePublication(t, s, r, f, p)
			switch mode {
			case "wrong account":
				p.account = "account2"
			case "source changed":
				r.Name = "Changed"
				if _, err := s.db.Exec(`UPDATE mail_filter_rules SET name=? WHERE id=?`, r.Name, r.ID); err != nil {
					t.Fatal(err)
				}
			case "provider changed":
				p.filters = []gmail.Filter{{ID: "new", Criteria: gmail.FilterCriteria{From: "elsewhere"}}}
			case "already exists":
				f.ID = "external"
				p.filters = []gmail.Filter{f}
			default:
				state := map[string]string{"pending job": "pending", "processing job": "processing", "unknown job": "outcome_unknown"}[mode]
				_, err := s.db.Exec(`INSERT INTO automatic_filter_jobs(id,rule_id,destination_folder_id,message_id,source_account_id,source_folder_id,source_uid,state) VALUES('job',?,'folder2','m','account1','folder1',1,?)`, r.ID, state)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
				t.Fatal("guard bypassed")
			}
			if p.writes != 0 {
				t.Fatal("provider contacted for write")
			}
		})
	}
}
func TestGmailPublicationRejectsUnsupportedDrafts(t *testing.T) {
	for _, mode := range []string{"enabled", "all accounts", "folder scope", "cross account move", "forward", "unknown label", "conflicting labels", "empty criteria", "pending job"} {
		t.Run(mode, func(t *testing.T) {
			s, r, f, p := publicationFixture(t)
			switch mode {
			case "enabled":
				r.Enabled = true
			case "all accounts":
				r.AccountIDs = nil
			case "folder scope":
				r.FolderIDs = []string{"folder1"}
			case "cross account move":
				r.DestFolderID = "folder3"
			case "forward":
				f.Action.Forward = "elsewhere@example.test"
			case "unknown label":
				f.Action.AddLabelIDs = []string{"label-other-account"}
			case "conflicting labels":
				f.Action.AddLabelIDs = []string{"UNREAD"}
			case "empty criteria":
				f.Criteria = gmail.FilterCriteria{}
			case "pending job":
				_, err := s.db.Exec(`INSERT INTO automatic_filter_jobs(id,rule_id,destination_folder_id,message_id,source_account_id,source_folder_id,source_uid,state) VALUES('job',?,'folder2','m','account1','folder1',1,'pending')`, r.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Update(r); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PrepareGmailPublication(r.ID, "account1", f, p.filters, []string{"UNREAD"}); err == nil {
				t.Fatal("unsupported draft accepted")
			}
		})
	}
}
