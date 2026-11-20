package mailfilter

import (
	"context"
	"errors"
	"testing"
)

type fakeTargetStore struct {
	rules     []*Rule
	writes    int
	reads     int
	failRead  int
	failWrite bool
	drop      bool
}

func (s *fakeTargetStore) List(context.Context) ([]*Rule, error) {
	s.reads++
	if s.reads == s.failRead {
		return nil, errors.New("offline")
	}
	return s.rules, nil
}
func (s *fakeTargetStore) Create(_ context.Context, r *Rule) error {
	s.writes++
	if s.failWrite {
		return errors.New("timeout")
	}
	if !s.drop {
		s.rules = []*Rule{r}
	}
	return nil
}
func (s *fakeTargetStore) Update(c context.Context, r *Rule) error { return s.Create(c, r) }
func TestSaveVerifiedRule(t *testing.T) {
	for _, tc := range []struct {
		name      string
		store     fakeTargetStore
		expected  *Rule
		wantError bool
		writes    int
	}{
		{name: "create read back", writes: 1},
		{name: "before read fails", store: fakeTargetStore{failRead: 1}, wantError: true},
		{name: "duplicate ID is conflict", store: fakeTargetStore{rules: []*Rule{{ID: "one", Name: "other"}}}, wantError: true},
		{name: "external update conflict", store: fakeTargetStore{rules: []*Rule{{ID: "one", Name: "other"}}}, expected: &Rule{ID: "one", Name: "before"}, wantError: true},
		{name: "missing existing conflict", expected: &Rule{ID: "one"}, wantError: true},
		{name: "update with baseline", store: fakeTargetStore{rules: []*Rule{{ID: "one", Name: "before"}}}, expected: &Rule{ID: "one", Name: "before"}, writes: 1},
		{name: "ambiguous write not retried", store: fakeTargetStore{failWrite: true}, writes: 1, wantError: true},
		{name: "readback failure is not success", store: fakeTargetStore{failRead: 2}, writes: 1, wantError: true},
		{name: "missing readback is not success", store: fakeTargetStore{drop: true}, writes: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Rule{ID: "one", Name: "new", Enabled: true, ActionMarkRead: true}
			err := SaveVerifiedRule(context.Background(), &tc.store, r, tc.expected)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if tc.store.writes != tc.writes {
				t.Fatalf("writes=%d", tc.store.writes)
			}
		})
	}
}

func TestSameRuleConfigurationNormalizesEmptySlices(t *testing.T) {
	a := &Rule{ID: "one", Name: "rule", FromContains: []string{"sender"}}
	b := *a
	b.AccountIDs = []string{}
	b.FolderIDs = []string{}
	b.ToContains = []string{}
	if !SameRuleConfiguration(a, &b) {
		t.Fatal("JSON null versus [] changed effective configuration")
	}
	b.FromContains = []string{"different"}
	if SameRuleConfiguration(a, &b) {
		t.Fatal("actual criterion change ignored")
	}
}

type persistentTargetStore struct{ store *Store }

func (s persistentTargetStore) List(context.Context) ([]*Rule, error)   { return s.store.List() }
func (s persistentTargetStore) Create(_ context.Context, r *Rule) error { return s.store.Create(r) }
func (s persistentTargetStore) Update(_ context.Context, r *Rule) error { return s.store.Update(r) }
func TestSaveVerifiedRuleSQLiteRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := persistentTargetStore{NewStore(db)}
	r := &Rule{ID: "verified", Name: "  Trimmed name  ", Enabled: true, ActionMarkRead: true, FromContains: []string{"sender"}}
	if err := SaveVerifiedRule(context.Background(), store, r, nil); err != nil {
		t.Fatal(err)
	}
	before, err := store.store.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated := *before
	updated.Name = "Updated"
	if err := SaveVerifiedRule(context.Background(), store, &updated, before); err != nil {
		t.Fatal(err)
	}
	if err := SaveVerifiedRule(context.Background(), store, r, before); err == nil {
		t.Fatal("stale baseline accepted")
	}
}
