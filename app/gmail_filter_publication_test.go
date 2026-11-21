package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
)

func TestNativePublicationRejectsAnyHelperOwner(t *testing.T) {
	for _, endpoint := range []string{"", "http://127.0.0.1:8029", "https://studio.example.test"} {
		a := &App{mailboxService: &mailboxServiceConnector{endpoint: endpoint}}
		if err := a.requireStandaloneFilterOwner(); err == nil {
			t.Fatal("helper ownership accepted")
		}
		if _, err := a.prepareLocalGmailPublication(context.Background(), "source", "mail", gmail.Filter{}); err == nil {
			t.Fatal("helper fallback")
		}
	}
}

func TestNativePublicationBoundClientAndExecutionGuard(t *testing.T) {
	a, db := contactOAuthFixture(t)
	oldClient := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(id string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: "test-" + id}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = oldClient })
	tokens, err := a.credStore.GetOAuthTokens("mail")
	if err != nil {
		t.Fatal(err)
	}
	tokens.Scopes = append(tokens.Scopes, gmail.ScopeSettingsBasic)
	if err = a.credStore.SetOAuthTokens("mail", tokens); err != nil {
		t.Fatal(err)
	}
	a.filterStore = mailfilter.NewStore(db)
	a.composeOps = composeOps{accountStore: a.accountStore, credStore: a.credStore}
	rule := &mailfilter.Rule{ID: "source", Name: "Rule", AccountIDs: []string{"mail"}, FromContains: []string{"sender"}, ActionMarkRead: true}
	if err = a.filterStore.Create(rule); err != nil {
		t.Fatal(err)
	}
	draft := gmail.Filter{Criteria: gmail.FilterCriteria{From: "sender@example.test"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	var writes atomic.Int32
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer mail-access" {
			t.Error("wrong account token")
		}
		body := ""
		switch {
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/labels"):
			body = `{"labels":[{"id":"UNREAD"}]}`
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/settings/filters"):
			body = `{"filter":[]}`
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/settings/filters"):
			writes.Add(1)
			body = `{"id":"remote","criteria":{"from":"sender@example.test"},"action":{"removeLabelIds":["UNREAD"]}}`
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/settings/filters/remote"):
			body = `{"id":"remote","criteria":{"from":"sender@example.test"},"action":{"removeLabelIds":["UNREAD"]}}`
		default:
			t.Error("unexpected provider request", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	// Holding a direct-filter read lease emulates an in-flight filter action.
	a.filterExecutionMu.RLock()
	done := make(chan *mailfilter.GmailPublication, 1)
	errs := make(chan error, 1)
	go func() {
		p, err := a.prepareLocalGmailPublication(context.Background(), rule.ID, "mail", draft)
		done <- p
		errs <- err
	}()
	select {
	case <-done:
		t.Fatal("prepared before direct filtering drained")
	case <-time.After(30 * time.Millisecond):
	}
	a.filterExecutionMu.RUnlock()
	plan := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if _, err := a.applyFilterActions(rule, []string{"message"}); err == nil {
		t.Fatal("queued direct callback bypassed publication guard")
	}
	published, err := a.publishLocalGmailPublication(context.Background(), plan.ID)
	if err != nil || published.ProviderID != "remote" {
		t.Fatal(published, err)
	}
	if _, err := a.publishLocalGmailPublication(context.Background(), plan.ID); err == nil {
		t.Fatal("published twice")
	}
	if writes.Load() != 1 {
		t.Fatal("wrong write count", writes.Load())
	}
	if _, err := a.reconcileLocalGmailPublication(context.Background(), plan.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNativePublicationRequiresExplicitGrantAndSelectedAccount(t *testing.T) {
	a, _ := contactOAuthFixture(t)
	if _, err := a.gmailFilterProvider("mail"); err == nil {
		t.Fatal("ordinary mail grant accepted")
	}
	if _, err := a.gmailFilterProvider("missing"); err == nil {
		t.Fatal("missing account accepted")
	}
}

func TestFilterBatchStopsAfterShutdownStarts(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ownFlagChangeAt = make(map[string]time.Time)
	// The read event is a deterministic point inside the first message action.
	// Starting shutdown there must prevent a second message in the same batch.
	f.app.runtimeEventEmitter = func(name string, _ ...interface{}) {
		if name == "messages:readChanged" {
			f.app.syncOwnerMu.Lock()
			f.app.syncShutdown = true
			f.app.syncOwnerMu.Unlock()
		}
	}
	rule := &mailfilter.Rule{ID: "batch", AccountIDs: []string{"acct-a"}, FromContains: []string{"bob"}, ActionMarkRead: true}
	n, err := f.app.applyFilterActions(rule, []string{f.message.ID}, f.message, f.message)
	if n != 1 || err == nil {
		t.Fatalf("batch continued during shutdown: count=%d err=%v", n, err)
	}
}

func TestFilterDrainWaitsForBackgroundMoveCompletion(t *testing.T) {
	a := &App{}
	m := &message.Message{ID: "one", AccountID: "mail"}
	terminal := make(chan struct{})
	a.moveMutationFlights = map[string]*moveMutationFlight{"mail\x00one": {terminal: terminal}}
	a.filterExecutionMu.RLock()
	waiting := make(chan struct{})
	done := make(chan struct{})
	go func() { close(waiting); a.waitForFilterMove(m); a.filterExecutionMu.RUnlock(); close(done) }()
	<-waiting
	acquired := make(chan struct{})
	go func() { a.filterExecutionMu.Lock(); close(acquired); a.filterExecutionMu.Unlock() }()
	select {
	case <-acquired:
		t.Fatal("publication drained before physical move")
	case <-time.After(20 * time.Millisecond):
	}
	close(terminal)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("filter did not join completion")
	}
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("publication never drained")
	}
}
