package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailfilter"
	"github.com/hkdb/aerion/internal/oauth2"
)

func nativeReviewFixture(t *testing.T) (*App, *mailfilter.Rule) {
	a, db := contactOAuthFixture(t)
	a.filterStore = mailfilter.NewStore(db)
	a.composeOps = composeOps{accountStore: a.accountStore, credStore: a.credStore}
	old := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(id string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: "test-" + id}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = old })
	tokens, err := a.credStore.GetOAuthTokens("mail")
	if err != nil {
		t.Fatal(err)
	}
	tokens.Scopes = append(tokens.Scopes, gmail.ScopeSettingsBasic)
	if err = a.credStore.SetOAuthTokens("mail", tokens); err != nil {
		t.Fatal(err)
	}
	rule := &mailfilter.Rule{ID: "source", Name: "Rule", AccountIDs: []string{"mail"}, FromContains: []string{"sender"}, ActionMarkRead: true}
	if err = a.filterStore.Create(rule); err != nil {
		t.Fatal(err)
	}
	return a, rule
}
func TestNativeGmailReviewPayload(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draft NativeGmailDraft
		bad   bool
	}{
		{"no query", NativeGmailDraft{MarkRead: true}, true},
		{"query control", NativeGmailDraft{Query: "a\nb", MarkRead: true}, true},
		{"empty action", NativeGmailDraft{Query: "from:sender"}, true},
		{"opposing relocate", NativeGmailDraft{Query: "from:sender", Archive: true, Trash: true}, true},
		{"native query preserved", NativeGmailDraft{Query: "from:(a OR b) has:attachment", MarkRead: true, Archive: true, LabelIDs: []string{"opaque-user-id"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := nativeGmailPayload(tc.draft)
			if (err != nil) != tc.bad {
				t.Fatal(err)
			}
			if !tc.bad && f.Criteria.Query != tc.draft.Query {
				t.Fatal("query semantics changed")
			}
		})
	}
}
func TestNativeGmailReviewBoundedPreviewAndFingerprint(t *testing.T) {
	a, r := nativeReviewFixture(t)
	writes := 0
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := ""
		if req.Method != "GET" {
			writes++
			t.Error("preview mutated provider")
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "/labels"):
			body = `{"labels":[{"id":"opaque-user-id","name":"Reading","type":"user"},{"id":"UNREAD","name":"Unread","type":"system"},{"id":"INBOX","name":"Inbox","type":"system"}]}`
		case strings.HasSuffix(req.URL.Path, "/messages"):
			if req.URL.Query().Get("maxResults") != "51" || req.URL.Query().Get("q") != "from:sender" || req.URL.Query().Get("includeSpamTrash") != "true" {
				t.Error("incorrect bounded provider query")
			}
			ids := []string{}
			for i := range 51 {
				ids = append(ids, fmt.Sprintf(`{"id":"m%d"}`, i))
			}
			body = `{"messages":[` + strings.Join(ids, ",") + `],"nextPageToken":"more","resultSizeEstimate":9999}`
		case strings.HasSuffix(req.URL.Path, "/settings/filters"):
			body = `{"filter":[]}`
		default:
			t.Error("unexpected request", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	ctx, err := a.GetNativeGmailContext(r.ID)
	if err != nil || ctx.AccountID != "mail" || len(ctx.Labels) != 1 {
		t.Fatal(ctx, err)
	}
	draft := NativeGmailDraft{RuleID: r.ID, Query: "from:sender", MarkRead: true, Archive: true, LabelIDs: []string{"opaque-user-id"}}
	preview, err := a.PreviewNativeGmailFilter(draft)
	if err != nil || preview.Found != 50 || !preview.Truncated {
		t.Fatal(preview, err)
	}
	changed := draft
	changed.Query = "from:other"
	if _, err = a.PrepareNativeGmailFilter(changed, preview.Fingerprint); err == nil {
		t.Fatal("changed draft accepted")
	}
	plan, err := a.PrepareNativeGmailFilter(draft, preview.Fingerprint)
	if err != nil || plan.Fingerprint != preview.Fingerprint || plan.State != "prepared" {
		t.Fatal(plan, err)
	}
	recovered, err := a.GetNativeGmailPublication(r.ID)
	if err != nil || recovered.ID != plan.ID || recovered.Query != draft.Query {
		t.Fatal(recovered, err)
	}
	if _, err = a.PublishNativeGmailFilter(plan.ID, "wrong fingerprint"); err == nil {
		t.Fatal("unreviewed payload published")
	}
	if err = a.CancelNativeGmailFilter(plan.ID); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatal("provider writes in preview/review")
	}
	invalid := draft
	invalid.LabelIDs = []string{"UNREAD"}
	if _, err = a.PreviewNativeGmailFilter(invalid); err == nil {
		t.Fatal("system label masqueraded as user label")
	}
}
func TestNativeGmailPreviewFailureNeverPrepares(t *testing.T) {
	a, r := nativeReviewFixture(t)
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	if _, err := a.PreviewNativeGmailFilter(NativeGmailDraft{RuleID: r.ID, Query: "sender", MarkRead: true}); err == nil {
		t.Fatal("failed preview accepted")
	}
	state, err := a.GetNativeGmailPublication(r.ID)
	if err != nil || state.ID != "" {
		t.Fatal("preview reserved source", state, err)
	}
}

func TestNativeGmailFingerprintSurvivesDurableCanonicalization(t *testing.T) {
	for _, draft := range []NativeGmailDraft{
		{Query: "from:sender", MarkRead: true},
		{Query: "from:sender", Trash: true},
		{Query: "from:sender", LabelIDs: []string{"b", "a", "b"}},
	} {
		t.Run(fmt.Sprint(draft.MarkRead, draft.Trash, draft.LabelIDs), func(t *testing.T) {
			a, r := nativeReviewFixture(t)
			f, err := nativeGmailPayload(draft)
			if err != nil {
				t.Fatal(err)
			}
			before := nativeGmailFingerprint(r, "mail", f)
			p, err := a.filterStore.PrepareGmailPublication(r.ID, "mail", f, nil, []string{"UNREAD", "TRASH", "a", "b"})
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := a.filterStore.GetGmailPublication(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if before == "" || publicationStatus(reopened).Fingerprint != before {
				t.Fatal("durable canonicalization changed reviewed payload")
			}
		})
	}
}
