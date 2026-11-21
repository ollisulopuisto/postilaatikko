package mailfilter

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
)

func TestPreparedGmailPublicationGuardsAndCancel(t *testing.T) {
	s, r, f, p := publicationFixture(t)
	plan := preparePublication(t, s, r, f, p)
	r.Name = "edited"
	if err := s.Update(r); err == nil {
		t.Fatal("source editable while publication guarded")
	}
	if err := s.Reorder([]string{r.ID}); err != nil {
		t.Fatal("unchanged guarded source blocked reorder", err)
	}
	if err := s.Reorder([]string{r.ID, r.ID}); err == nil {
		t.Fatal("guarded source moved")
	}
	if err := s.Delete(r.ID); err == nil {
		t.Fatal("guarded rule deletion reported success")
	}
	if _, err := s.PrepareGmailPublication(r.ID, "account1", f, nil, []string{"UNREAD"}); err == nil {
		t.Fatal("second plan accepted")
	}
	msg := &message.Message{ID: "m", AccountID: "account1", FolderID: "folder1", UID: 1}
	if err := NewJobStore(s.db).Enqueue(r, "", msg, true); err == nil {
		t.Fatal("guarded local job accepted")
	}
	if err := s.CancelGmailPublication(plan.ID); err != nil {
		t.Fatal(err)
	}
	r.Enabled = true
	if err := s.Update(r); err != nil {
		t.Fatal("cancel did not release source", err)
	}
}
func TestAttemptedGmailPublicationCannotCancel(t *testing.T) {
	s, r, f, p := publicationFixture(t)
	p.uncertain = true
	plan := preparePublication(t, s, r, f, p)
	if _, err := s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
		t.Fatal("expected uncertainty")
	}
	if err := s.CancelGmailPublication(plan.ID); err == nil {
		t.Fatal("attempt history removed")
	}
}
func TestGmailPublicationSurvivesActualDatabaseRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('account1','Work','a@example.test','imap','smtp','a')`); err != nil {
		t.Fatal(err)
	}
	_, r, f, p := publicationFixture(t)
	if err = s.Create(r); err != nil {
		t.Fatal(err)
	}
	plan := preparePublication(t, s, r, f, p)
	// Simulate process death after request submission, before saving its ID.
	if _, err = db.Exec(`UPDATE gmail_filter_publications SET state='attempted' WHERE id=?`, plan.ID); err != nil {
		t.Fatal(err)
	}
	created := f
	created.ID = "remote-created"
	p.filters = append(p.filters, created)
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = NewStore(db)
	result, err := s.ReconcileGmailPublication(context.Background(), plan.ID, p)
	if err != nil || result.ProviderID != "remote-created" {
		t.Fatal(result, err)
	}
	if _, err = s.PublishGmailFilter(context.Background(), plan.ID, p); err == nil {
		t.Fatal("restarted create replayed")
	}
	if p.writes != 0 {
		t.Fatal("reconciliation made a write")
	}
}
func TestConcurrentGmailPublicationClaimsAtMostOnce(t *testing.T) {
	s, r, f, p := publicationFixture(t)
	plan := preparePublication(t, s, r, f, p)
	// Serialize fake-provider access, like one bound account client.
	provider := &lockedNativeProvider{p: p}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.PublishGmailFilter(context.Background(), plan.ID, provider) }()
	}
	wg.Wait()
	if p.writes != 1 {
		t.Fatalf("writes=%d", p.writes)
	}
}

type lockedNativeProvider struct {
	mu sync.Mutex
	p  *fakeNativeProvider
}

func (p *lockedNativeProvider) AccountID() string { return p.p.account }
func (p *lockedNativeProvider) ListFilters(c context.Context) ([]gmail.Filter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	items, err := p.p.ListFilters(c)
	return append([]gmail.Filter(nil), items...), err
}
func (p *lockedNativeProvider) CreateFilter(c context.Context, f gmail.Filter) (*gmail.Filter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.p.CreateFilter(c, f)
}
func (p *lockedNativeProvider) GetFilter(c context.Context, id string) (*gmail.Filter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.p.GetFilter(c, id)
}

func TestReorderOtherRulesPreservesGmailSourceSnapshot(t *testing.T) {
	s, r, f, p := publicationFixture(t)
	plan := preparePublication(t, s, r, f, p)
	for _, id := range []string{"other-a", "other-b"} {
		rule := *r
		rule.ID = id
		rule.Name = id
		rule.OrderIndex = 1
		if err := s.Create(&rule); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reorder([]string{r.ID, "other-b", "other-a"}); err != nil {
		t.Fatal("unrelated reorder blocked", err)
	}
	source, err := s.Get(r.ID)
	if err != nil || !SameRuleConfiguration(source, &plan.Source) {
		t.Fatal("source snapshot changed", err)
	}
	if err := s.Reorder([]string{"other-b", r.ID, "other-a"}); err == nil {
		t.Fatal("guarded source moved")
	}
	source, err = s.Get(r.ID)
	if err != nil || !SameRuleConfiguration(source, &plan.Source) {
		t.Fatal("failed reorder changed source", err)
	}
}
