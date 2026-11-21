package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	automationclient "github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
)

func TestNativeGmailReviewPinsHelperWithoutLocalFallback(t *testing.T) {
	var calls []v1.GmailPublicationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/filters/gmail" {
			t.Error("wrong route", r.URL.Path)
		}
		var req v1.GmailPublicationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, req)
		w.Header().Set("Content-Type", "application/json")
		if req.Operation == "context" {
			fmt.Fprint(w, `{"data":{"accountId":"helper-account","email":"helper@example.com","consentRequired":true}}`)
			return
		}
		if req.Operation == "preview" {
			fmt.Fprint(w, `{"data":{"email":"helper@example.com","found":50,"truncated":true,"fingerprint":"helper-fingerprint"}}`)
			return
		}
		if req.Operation == "publish" {
			http.Error(w, "outcome unknown", 503)
			return
		}
		fmt.Fprint(w, `{"data":{"accountId":"helper-account","fingerprint":"helper-fingerprint","publication":{"id":"attempt","ruleId":"source","accountId":"helper-account","state":"unknown","ownerBinding":"[\"helper@example.com\",\"project\"]","filter":{"criteria":{"query":"from:reviewed"},"action":{"removeLabelIds":["UNREAD"]}}}}}`)
	}))
	defer server.Close()
	app := &App{ctx: context.Background(), mailboxService: &mailboxServiceConnector{endpoint: server.URL, client: automationclient.New(automationclient.Config{Endpoint: server.URL})}}
	workspace, err := app.GetNativeGmailWorkspace()
	if err != nil || workspace.Target != "helper" || workspace.Endpoint != server.URL {
		t.Fatal(workspace, err)
	}
	reply, err := app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "context", RuleID: "source"})
	if err != nil || reply.Context == nil || reply.Context.AccountID != "helper-account" {
		t.Fatal(reply, err)
	}
	reply, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "preview", RuleID: "source", AccountID: "helper-account", Draft: NativeGmailDraft{RuleID: "source", Query: "from:reviewed", MarkRead: true}})
	if err != nil || reply.Preview == nil || reply.Preview.Fingerprint != "helper-fingerprint" || !reply.Preview.Truncated {
		t.Fatal(reply, err)
	}
	last := calls[len(calls)-1]
	if last.AccountID != "helper-account" || last.Filter.Criteria.Query != "from:reviewed" || len(last.Filter.Action.RemoveLabelIDs) != 1 {
		t.Fatal("payload changed", last)
	}
	reply, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "status", RuleID: "source"})
	if err != nil || reply.Publication == nil || reply.Publication.Fingerprint != "helper-fingerprint" || reply.Publication.AccountEmail != "helper@example.com" {
		t.Fatal(reply, err)
	}
	before := len(calls)
	_, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "publish", ID: "attempt", RuleID: "source", AccountID: "helper-account", Fingerprint: "helper-fingerprint"})
	if err == nil || len(calls) != before+1 {
		t.Fatal("publish retried or failed open", err)
	}
	// Replacing the same endpoint's credential/client invalidates an open review.
	app.mailboxService.client = automationclient.New(automationclient.Config{Endpoint: server.URL})
	before = len(calls)
	if _, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "status", RuleID: "source"}); err == nil || len(calls) != before {
		t.Fatal("changed client reached helper", err)
	}
	app.mailboxService = nil
	if _, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "context", RuleID: "source"}); err == nil {
		t.Fatal("helper review fell back to local")
	}
}

func TestNativeGmailReviewRejectsDraftMismatchBeforeHelper(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "offline", 503) }))
	defer server.Close()
	app := &App{ctx: context.Background(), mailboxService: &mailboxServiceConnector{endpoint: server.URL, client: automationclient.New(automationclient.Config{Endpoint: server.URL})}}
	workspace, err := app.GetNativeGmailWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "preview", RuleID: "source", Draft: NativeGmailDraft{RuleID: "different", Query: "from:test", MarkRead: true}})
	if err == nil || calls != 0 {
		t.Fatal("mismatched source reached helper", err)
	}
	_, err = app.NativeGmailReview(NativeGmailReviewRequest{Workspace: workspace.Key, Operation: "context", RuleID: "source"})
	if err == nil || calls != 1 {
		t.Fatal("offline helper did not fail closed", err)
	}
}

func TestNativeGmailConsentForwardsOnlyDesktopCallback(t *testing.T) {
	var requests []v1.GmailConsentRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req v1.GmailConsentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		requests = append(requests, req)
		reply := v1.GmailConsentReply{AccountID: req.AccountID, Email: "helper@example.com"}
		switch req.Operation {
		case "start":
			reply.SessionID = "session"
			reply.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth?state=bound-state&redirect_uri=" + url.QueryEscape(req.RedirectURI)
		case "complete":
			if req.Code != "one-code" || req.State != "bound-state" || req.SessionID != "session" {
				t.Error("callback changed")
			}
			reply.Authorized = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.Response[v1.GmailConsentReply]{Data: reply})
	}))
	defer server.Close()
	app := &App{ctx: context.Background(), mailboxService: &mailboxServiceConnector{endpoint: server.URL, client: automationclient.New(automationclient.Config{Endpoint: server.URL})}}
	workspace, err := app.GetNativeGmailWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	err = app.authorizeNativeGmailWorkspace(workspace.Key, "helper-account", func(email, authURL string) {
		opened++
		if email != "helper@example.com" {
			t.Error("wrong email")
		}
		u, e := url.Parse(authURL)
		if e != nil {
			t.Error(e)
			return
		}
		go func() {
			resp, e := http.Get(u.Query().Get("redirect_uri") + "?code=one-code&state=bound-state")
			if e != nil {
				t.Error(e)
				return
			}
			resp.Body.Close()
		}()
	})
	if err != nil || opened != 1 || len(requests) != 3 || requests[0].Operation != "start" || requests[1].Operation != "complete" || requests[2].Operation != "cancel" {
		t.Fatal("consent sequence failed", err, opened, len(requests))
	}
}

func TestNativeGmailConsentRejectsUnexpectedBrowserDestination(t *testing.T) {
	cancelled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req v1.GmailConsentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Operation == "cancel" {
			cancelled = true
			fmt.Fprint(w, `{"data":{"accountId":"account"}}`)
			return
		}
		fmt.Fprint(w, `{"data":{"accountId":"account","sessionId":"session","authURL":"https://other.example.test/consent"}}`)
	}))
	defer server.Close()
	app := &App{ctx: context.Background(), mailboxService: &mailboxServiceConnector{endpoint: server.URL, client: automationclient.New(automationclient.Config{Endpoint: server.URL})}}
	workspace, err := app.GetNativeGmailWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	err = app.authorizeNativeGmailWorkspace(workspace.Key, "account", func(string, string) { opened = true })
	if err == nil || opened || !cancelled {
		t.Fatal("unexpected destination opened or retained session", err, opened, cancelled)
	}
}
