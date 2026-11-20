package app

import (
	"context"
	"encoding/json"
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativePublicationRejectsChangedOwnerAndProject(t *testing.T) {
	for _, change := range []string{"account", "project", "provider identity", "old unbound plan"} {
		t.Run(change, func(t *testing.T) {
			a, r := nativeReviewFixture(t)
			project := "initial-project"
			oldLookup := oauth2.UserOverrideLookup
			oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
				return oauth2.ClientCredentials{ClientID: project}, true
			}
			t.Cleanup(func() { oauth2.UserOverrideLookup = oldLookup })
			actual := "user@example.com"
			writes := 0
			oldTransport := http.DefaultTransport
			http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"filter":[]}`
				if strings.HasSuffix(req.URL.Path, "/profile") {
					body = `{"emailAddress":"` + actual + `"}`
				}
				if strings.HasSuffix(req.URL.Path, "/labels") {
					body = `{"labels":[{"id":"UNREAD","type":"system"}]}`
				}
				if req.Method == http.MethodPost {
					writes++
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			draft := gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
			p, err := a.prepareLocalGmailPublication(context.Background(), r.ID, "mail", draft)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := a.filterStore.GetGmailPublication(p.ID)
			if err != nil || recovered.OwnerBinding == "" || recovered.OwnerBinding != p.OwnerBinding {
				t.Fatal("owner binding not durable", recovered, err)
			}
			switch change {
			case "account":
				acc, readErr := a.accountStore.Get("mail")
				if readErr != nil {
					t.Fatal(readErr)
				}
				data, _ := json.Marshal(acc)
				var cfg account.AccountConfig
				if err = json.Unmarshal(data, &cfg); err == nil {
					cfg.Email = "different@example.com"
					cfg.DisplayName = "User"
					_, err = a.accountStore.Update("mail", &cfg)
				}
			case "project":
				project = "changed-project"
			case "provider identity":
				actual = "different@example.com"
			case "old unbound plan":
				if err = a.filterStore.CancelGmailPublication(p.ID); err == nil {
					p, err = a.filterStore.PrepareGmailPublication(r.ID, "mail", draft, nil, []string{"UNREAD"})
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.publishLocalGmailPublication(context.Background(), p.ID); err == nil {
				t.Fatal("changed or missing owner verification accepted")
			}
			if writes != 0 {
				t.Fatal("provider write after owner mismatch")
			}
		})
	}
}

func TestNativePublicationWritePinsVerifiedIdentityToken(t *testing.T) {
	a, _ := nativeReviewFixture(t)
	p, err := a.gmailFilterProvider("mail")
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"id":"created"}`
		if strings.HasSuffix(req.URL.Path, "/profile") {
			body = `{"emailAddress":"user@example.com"}`
			tokens, err := a.credStore.GetOAuthTokens("mail")
			if err != nil {
				t.Fatal(err)
			}
			tokens.AccessToken = "replacement-access"
			tokens.RefreshToken = "replacement-refresh"
			if err = a.credStore.SetOAuthTokens("mail", tokens); err != nil {
				t.Fatal(err)
			}
			tokens.Clear()
		}
		if req.Method == http.MethodPost {
			writes++
			if req.Header.Get("Authorization") != "Bearer mail-access" {
				t.Error("write switched away from verified token")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	_, err = p.CreateFilter(context.Background(), gmail.Filter{Criteria: gmail.FilterCriteria{Query: "sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}})
	if err != nil || writes != 1 {
		t.Fatal(writes, err)
	}
}

func TestNativePreviewFingerprintIncludesOAuthProject(t *testing.T) {
	a, r := nativeReviewFixture(t)
	project := "first-project"
	oldLookup := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: project}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = oldLookup })
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"messages":[]}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	draft := NativeGmailDraft{RuleID: r.ID, Query: "from:sender", MarkRead: true}
	preview, err := a.PreviewNativeGmailFilter(draft)
	if err != nil {
		t.Fatal(err)
	}
	project = "different-project"
	if _, err = a.PrepareNativeGmailFilter(draft, preview.Fingerprint); err == nil {
		t.Fatal("preview accepted after project changed")
	}
	status, err := a.GetNativeGmailPublication(r.ID)
	if err != nil || status.ID != "" {
		t.Fatal("changed project reserved local source", status, err)
	}
}

func TestNativePublicationIdentityFailureBeforePostKeepsPlanCancellable(t *testing.T) {
	a, rule := nativeReviewFixture(t)
	oldLookup := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: "project"}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = oldLookup })
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	profiles, writes := 0, 0
	fail := false
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"filter":[]}`
		status := 200
		if strings.HasSuffix(req.URL.Path, "/profile") {
			profiles++
			body = `{"emailAddress":"user@example.com"}`
			if fail && profiles > 1 {
				status = 400
				body = `{"error":{"message":"profile unavailable"}}`
			}
		}
		if strings.HasSuffix(req.URL.Path, "/labels") {
			body = `{"labels":[{"id":"UNREAD"}]}`
		}
		if req.Method == http.MethodPost {
			writes++
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	draft := gmail.Filter{Criteria: gmail.FilterCriteria{Query: "from:sender"}, Action: gmail.FilterAction{RemoveLabelIDs: []string{"UNREAD"}}}
	plan, err := a.prepareLocalGmailPublication(context.Background(), rule.ID, "mail", draft)
	if err != nil {
		t.Fatal(err)
	}
	profiles = 0
	fail = true
	if _, err = a.publishLocalGmailPublication(context.Background(), plan.ID); err == nil || writes != 0 {
		t.Fatal("identity preflight submitted write", err, writes)
	}
	stored, err := a.filterStore.GetGmailPublication(plan.ID)
	if err != nil || stored.State != "prepared" {
		t.Fatal("preflight consumed attempt", err)
	}
	if err = a.filterStore.CancelGmailPublication(plan.ID); err != nil {
		t.Fatal("source stranded", err)
	}
}
