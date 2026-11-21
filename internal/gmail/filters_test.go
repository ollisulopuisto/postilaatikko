package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func filterTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := NewClient(func(context.Context, bool) (string, error) { return "test-token", nil })
	c.BaseURL = server.URL
	return c
}
func TestFiltersCRUD(t *testing.T) {
	calls := 0
	filter := Filter{ID: "provider-id", Criteria: FilterCriteria{From: "sender@example.test"}, Action: FilterAction{RemoveLabelIDs: []string{"INBOX"}}}
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing grant")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/settings/filters":
			_ = json.NewEncoder(w).Encode(map[string]any{"filter": []Filter{filter}})
		case r.Method == http.MethodGet && r.URL.Path == "/settings/filters/provider-id":
			_ = json.NewEncoder(w).Encode(filter)
		case r.Method == http.MethodPost:
			var got Filter
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			if got.ID != "" || got.Criteria.From != filter.Criteria.From {
				t.Errorf("filter=%+v", got)
			}
			_ = json.NewEncoder(w).Encode(filter)
		case r.Method == http.MethodDelete:
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	listed, err := c.ListFilters(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v %v", listed, err)
	}
	if _, err = c.GetFilter(ctx, filter.ID); err != nil {
		t.Fatal(err)
	}
	newFilter := filter
	newFilter.ID = ""
	created, err := c.CreateFilter(ctx, newFilter)
	if err != nil || created.ID == "" {
		t.Fatalf("create=%v %v", created, err)
	}
	if err = c.DeleteFilter(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestCreateFilterUnknownOutcomeNeverRetries(t *testing.T) {
	for _, status := range []int{200, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = w.Write([]byte("{"))
			})
			_, err := c.CreateFilter(context.Background(), Filter{Criteria: FilterCriteria{Query: "from:sender"}, Action: FilterAction{RemoveLabelIDs: []string{"INBOX"}}})
			if !errors.Is(err, ErrFilterOutcomeUnknown) || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestFilterScopeIsExplicit(t *testing.T) {
	for _, scopes := range [][]string{nil, {scopeFullMail}, {scopeModify}, {scopeReadonly}} {
		if ScopesCoverFilters(scopes) {
			t.Fatalf("unexpected settings access for %v", scopes)
		}
	}
	if !ScopesCoverFilters([]string{ScopeSettingsBasic}) {
		t.Fatal("explicit grant rejected")
	}
}
func TestInvalidFilterIDNeverCallsProvider(t *testing.T) {
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid ID reached provider") })
	for _, id := range []string{"", "../other", "one?two", "one#two"} {
		if _, err := c.GetFilter(context.Background(), id); err == nil {
			t.Fatal("invalid ID accepted")
		}
		if err := c.DeleteFilter(context.Background(), id); err == nil {
			t.Fatal("invalid ID accepted")
		}
	}
}
func TestCreateFilterDefiniteScopeFailure(t *testing.T) {
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"insufficientPermissions"}]}}`))
	})
	_, err := c.CreateFilter(context.Background(), Filter{Criteria: FilterCriteria{Query: "from:sender"}, Action: FilterAction{RemoveLabelIDs: []string{"INBOX"}}})
	if !errors.Is(err, ErrInsufficientScope) || errors.Is(err, ErrFilterOutcomeUnknown) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateRejectsForwardingButReadRetainsIt(t *testing.T) {
	calls := 0
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet {
			t.Fatal("forwarding write reached provider")
		}
		_ = json.NewEncoder(w).Encode(Filter{ID: "existing", Action: FilterAction{Forward: "verified@example.test"}})
	})
	for _, labels := range [][]string{nil, {"INBOX"}} {
		_, err := c.CreateFilter(context.Background(), Filter{Criteria: FilterCriteria{Query: "from:sender"}, Action: FilterAction{Forward: "verified@example.test", RemoveLabelIDs: labels}})
		if err == nil {
			t.Fatal("forwarding create accepted")
		}
	}
	if calls != 0 {
		t.Fatal("rejected create contacted provider")
	}
	existing, err := c.GetFilter(context.Background(), "existing")
	if err != nil || existing.Action.Forward != "verified@example.test" {
		t.Fatal("existing forwarding lost", existing, err)
	}
}

func TestFilterWriteErrorDistinguishesRejectionFromUncertainty(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 503} {
		err := filterWriteError(&APIError{Status: status})
		if errors.Is(err, ErrFilterRejected) != (status < 500) || errors.Is(err, ErrFilterOutcomeUnknown) != (status >= 500) {
			t.Fatal(status, err)
		}
	}
}

func TestFilterCreateTokenFailureNeverSubmits(t *testing.T) {
	calls := 0
	c := filterTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) })
	c.tokenSource = func(context.Context, bool) (string, error) { return "", errors.New("owner shutting down") }
	_, err := c.CreateFilter(context.Background(), Filter{Criteria: FilterCriteria{Query: "from:a"}, Action: FilterAction{RemoveLabelIDs: []string{"UNREAD"}}})
	if !errors.Is(err, ErrFilterNotSubmitted) || errors.Is(err, ErrFilterOutcomeUnknown) || calls != 0 {
		t.Fatal(err, calls)
	}
}
