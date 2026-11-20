package alwayson

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	gokeyring "github.com/zalando/go-keyring"
)

func helperNativeFixture(t *testing.T) *SyncRuntime {
	t.Helper()
	gokeyring.MockInit()
	r := openContactsAPITestRuntime(t)
	_, err := r.db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type) VALUES('helper-google','Helper','helper@example.com','imap.gmail.com','smtp.gmail.com','helper@example.com','oauth2')`)
	if err != nil {
		t.Fatal(err)
	}
	err = r.contacts.creds.SetOAuthTokens("helper-google", &credentials.OAuthTokens{Provider: "google", AccessToken: "helper-access", RefreshToken: "helper-refresh", ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{"https://mail.google.com/", gmail.ScopeSettingsBasic}})
	if err != nil {
		t.Fatal(err)
	}
	old := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: "helper-project"}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = old })
	if err = r.rules.Create(&mailfilter.Rule{ID: "helper-rule", AccountIDs: []string{"helper-google"}, Name: "Stopped", ActionMarkRead: true, FromContains: []string{"sender"}}); err != nil {
		t.Fatal(err)
	}
	err = r.SetGmailFilterTokenSource(func(_ context.Context, id string, force bool) (string, error) {
		if id != "helper-google" || force {
			return "", fmt.Errorf("wrong helper token request")
		}
		return "helper-access", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type helperNativeRoundTrip func(*http.Request) (*http.Response, error)

func (f helperNativeRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func TestHelperNativePublicationOneAttemptAndReadOnlyRecovery(t *testing.T) {
	r := helperNativeFixture(t)
	writes := 0
	actual := "helper@example.com"
	created := false
	oldTransport := http.DefaultTransport
	http.DefaultTransport = helperNativeRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer helper-access" {
			t.Error("desktop/wrong account token reached helper")
		}
		body := ""
		switch {
		case strings.HasSuffix(req.URL.Path, "/profile"):
			body = `{"emailAddress":"` + actual + `"}`
		case strings.HasSuffix(req.URL.Path, "/labels"):
			body = `{"labels":[{"id":"UNREAD","type":"system"},{"id":"INBOX","type":"system"},{"id":"TRASH","type":"system"},{"id":"opaque-user","type":"user","name":"Reading"}]}`
		case strings.HasSuffix(req.URL.Path, "/messages"):
			if req.URL.Query().Get("maxResults") != "51" || req.URL.Query().Get("q") != "from:sender" {
				t.Error("unbounded/changed preview")
			}
			ids := []string{}
			for i := range 51 {
				ids = append(ids, fmt.Sprintf(`{"id":"m%d"}`, i))
			}
			body = `{"messages":[` + strings.Join(ids, ",") + `],"resultSizeEstimate":99999}`
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/settings/filters"):
			writes++
			created = true
			return nil, fmt.Errorf("simulated response loss")
		case strings.HasSuffix(req.URL.Path, "/settings/filters"):
			body = `{"filter":[]}`
			if created {
				body = `{"filter":[{"id":"provider-rule","criteria":{"query":"from:sender"},"action":{"removeLabelIds":["UNREAD"]}}]}`
			}
		default:
			t.Error("unexpected request", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	req := v1.GmailPublicationRequest{Operation: "context", AccountID: "helper-google", RuleID: "helper-rule"}
	contextReply, err := r.GmailPublication(context.Background(), req)
	if err != nil || contextReply.Email != "helper@example.com" || contextReply.ConsentRequired || len(contextReply.Labels) != 1 {
		t.Fatal(contextReply, err)
	}
	req.Operation = "preview"
	req.Filter = gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	preview, err := r.GmailPublication(context.Background(), req)
	if err != nil || preview.Found != 50 || !preview.Truncated || writes != 0 {
		t.Fatal(preview, err)
	}
	req.Operation = "prepare"
	req.Fingerprint = preview.Fingerprint
	plan, err := r.GmailPublication(context.Background(), req)
	if err != nil || plan.Publication == nil || writes != 0 {
		t.Fatal(plan, err)
	}
	if plan.Publication.OwnerBinding == "" {
		t.Fatal("missing durable helper owner")
	}
	req.Operation = "publish"
	req.ID = plan.Publication.ID
	req.Fingerprint = "wrong"
	if _, err = r.GmailPublication(context.Background(), req); err == nil || writes != 0 {
		t.Fatal("unreviewed helper write")
	}
	req.Fingerprint = plan.Fingerprint
	if _, err = r.GmailPublication(context.Background(), req); err == nil || writes != 1 {
		t.Fatal("unknown create not retained", writes, err)
	}
	if _, err = r.GmailPublication(context.Background(), req); err == nil || writes != 1 {
		t.Fatal("helper create replayed", writes, err)
	}
	req.Operation = "status"
	status, err := r.GmailPublication(context.Background(), req)
	if err != nil || status.Publication.State != "unknown" {
		t.Fatal(status, err)
	}
	req.Operation = "reconcile"
	actual = "other@example.com"
	if _, err = r.GmailPublication(context.Background(), req); err == nil {
		t.Fatal("reconciled different Gmail identity")
	}
	actual = "helper@example.com"
	recovered, err := r.GmailPublication(context.Background(), req)
	if err != nil || recovered.Publication.State != "published" || writes != 1 {
		t.Fatal(recovered, writes, err)
	}
	req.AccountID = "wrong"
	if _, err = r.GmailPublication(context.Background(), req); err == nil {
		t.Fatal("wrong account accepted")
	}
	encoded, _ := json.Marshal(recovered)
	if strings.Contains(string(encoded), "helper-access") || strings.Contains(string(encoded), "helper-refresh") {
		t.Fatal("helper credentials returned through API")
	}
}
func TestHelperNativeMissingConsentDoesNotContactProvider(t *testing.T) {
	r := helperNativeFixture(t)
	tokens, err := r.contacts.creds.GetOAuthTokens("helper-google")
	if err != nil {
		t.Fatal(err)
	}
	tokens.Scopes = []string{"https://mail.google.com/"}
	if err = r.contacts.creds.SetOAuthTokens("helper-google", tokens); err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	http.DefaultTransport = helperNativeRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("provider called without explicit settings scope")
		return nil, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	req := v1.GmailPublicationRequest{Operation: "context", AccountID: "helper-google", RuleID: "helper-rule"}
	reply, err := r.GmailPublication(context.Background(), req)
	if err != nil || !reply.ConsentRequired {
		t.Fatal(reply, err)
	}
	req.Operation = "preview"
	if _, err = r.GmailPublication(context.Background(), req); err == nil {
		t.Fatal("missing settings grant accepted")
	}
}
func TestHelperNativeDraftRejectsImplicitConversionAndSystemLabelInjection(t *testing.T) {
	for _, f := range []gmail.Filter{
		{Criteria: gmail.FilterCriteria{From: "sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}},
		{Criteria: gmail.FilterCriteria{Query: "sender"}, Action: gmail.FilterAction{Forward: "other@example.com"}},
		{Criteria: gmail.FilterCriteria{Query: "sender"}, Action: gmail.FilterAction{AddLabelIDs: []string{"TRASH"}, RemoveLabelIDs: []string{"INBOX"}}},
		{Criteria: gmail.FilterCriteria{Query: "sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"user-label"}}},
	} {
		if _, err := helperNativeDraft(f); err == nil {
			t.Fatal("unsupported provider draft accepted", f)
		}
	}
	f := gmail.Filter{Criteria: gmail.FilterCriteria{Query: "sender"}, Action: gmail.FilterAction{AddLabelIDs: []string{"UNREAD"}}}
	if _, err := helperAllowedLabels(f, []gmail.Label{{ID: "UNREAD", Type: "system"}}); err == nil {
		t.Fatal("system label injected as user choice")
	}
}

func TestHelperNativePreparationRefusesOutstandingWorkAndFreezesSource(t *testing.T) {
	for _, state := range []string{"pending", "processing", "outcome_unknown", "none"} {
		t.Run(state, func(t *testing.T) {
			r := helperNativeFixture(t)
			old := http.DefaultTransport
			http.DefaultTransport = helperNativeRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"messages":[]}`
				if strings.HasSuffix(req.URL.Path, "/profile") {
					body = `{"emailAddress":"helper@example.com"}`
				}
				if strings.HasSuffix(req.URL.Path, "/labels") {
					body = `{"labels":[{"id":"UNREAD","type":"system"}]}`
				}
				if strings.HasSuffix(req.URL.Path, "/settings/filters") {
					body = `{"filter":[]}`
				}
				if req.Method != "GET" {
					t.Error("preparation wrote provider")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			req := v1.GmailPublicationRequest{Operation: "preview", AccountID: "helper-google", RuleID: "helper-rule", Filter: gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}}
			preview, err := r.GmailPublication(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if state != "none" {
				_, err = r.db.Exec(`INSERT INTO automatic_filter_jobs(id,rule_id,destination_folder_id,message_id,source_account_id,source_folder_id,source_uid,state) VALUES('job','helper-rule','','m','helper-google','f',1,?)`, state)
				if err != nil {
					t.Fatal(err)
				}
			}
			req.Operation = "prepare"
			req.Fingerprint = preview.Fingerprint
			result, err := r.GmailPublication(context.Background(), req)
			if state != "none" {
				if err == nil {
					t.Fatal("outstanding helper work allowed preparation")
				}
				return
			}
			if err != nil || result.Publication == nil {
				t.Fatal(result, err)
			}
			source, err := r.rules.Get("helper-rule")
			if err != nil {
				t.Fatal(err)
			}
			source.Enabled = true
			if err = r.rules.Update(source); err == nil {
				t.Fatal("prepared helper source enabled")
			}
			if err = r.rules.Delete("helper-rule"); err == nil {
				t.Fatal("prepared helper source deleted")
			}
			req.Operation = "cancel"
			req.ID = result.Publication.ID
			cancelled, err := r.GmailPublication(context.Background(), req)
			if err != nil || cancelled.Publication != nil {
				t.Fatal(cancelled, err)
			}
			if err = r.rules.Update(source); err != nil {
				t.Fatal("cancel did not release source", err)
			}
		})
	}
}

func TestProductionMailboxHandlerMountsNativeGmailRoute(t *testing.T) {
	r := openContactsAPITestRuntime(t)
	handler, err := r.MailboxReadAPIHandler()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail", strings.NewReader(`{"operation":"context","accountId":"helper"}`)))
	if rec.Code == 404 {
		t.Fatal("native Gmail ownership route missing")
	}
}

func TestHelperNativeReviewDiscoversOnlySourceOwnerAndAttempt(t *testing.T) {
	r := helperConsentFixture(t) // no settings consent, discovery makes no Google calls
	contextReply, err := r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "context", RuleID: "helper-rule"})
	if err != nil || contextReply.AccountID != "helper-google" || !contextReply.ConsentRequired {
		t.Fatal(contextReply, err)
	}
	status, err := r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "status", RuleID: "helper-rule"})
	if err != nil || status.Publication != nil {
		t.Fatal(status, err)
	}
	// Prepare a journal fixture directly, not a provider mutation.
	f := gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	plan, err := r.rules.PrepareGmailPublication("helper-rule", "helper-google", f, nil, []string{"UNREAD"}, helperGmailBinding("helper@example.com", "helper-project"))
	if err != nil {
		t.Fatal(err)
	}
	status, err = r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "status", RuleID: "helper-rule"})
	if err != nil || status.Publication == nil || status.Publication.ID != plan.ID || status.Fingerprint == "" {
		t.Fatal(status, err)
	}
	if _, err = r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "status", AccountID: "helper-google", ID: plan.ID, RuleID: "different-source"}); err == nil {
		t.Fatal("other source attempt disclosed")
	}
	if _, err = r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "context", RuleID: "missing"}); err == nil {
		t.Fatal("missing source inferred")
	}
}

func TestHelperNativeLostPreparationReplyRecoveredBySource(t *testing.T) {
	r := helperNativeFixture(t)
	draft := gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	original, err := r.rules.PrepareGmailPublication("helper-rule", "helper-google", draft, nil, []string{"UNREAD"}, helperGmailBinding("helper@example.com", "helper-project"))
	if err != nil {
		t.Fatal(err)
	}
	// Discard the prepare reply. Recover only from the original account/source.
	reply, err := r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "status", RuleID: "helper-rule", AccountID: "helper-google"})
	if err != nil || reply.Publication == nil || reply.Publication.ID != original.ID || reply.Fingerprint == "" {
		t.Fatal("lost reservation unrecoverable", reply, err)
	}
	if _, err = r.GmailPublication(context.Background(), v1.GmailPublicationRequest{Operation: "cancel", AccountID: "helper-google", ID: reply.Publication.ID}); err != nil {
		t.Fatal("recovered reservation not cancellable", err)
	}
}
