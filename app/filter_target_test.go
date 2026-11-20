package app

import (
	"context"
	"encoding/json"
	"fmt"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkdb/aerion/internal/mailfilter"
)

func TestFilterTargetNeverFallsBackFromRemote(t *testing.T) {
	for _, status := range []MailboxServiceStatus{
		{ErrorCode: "remote_unavailable"},
		{Connected: true, WriteAuthorized: false},
	} {
		a := &App{mailboxService: &mailboxServiceConnector{endpoint: "https://studio.example.test:8029", statusFunc: func(context.Context) MailboxServiceStatus { return status }}}
		target := a.GetFilterExecutionTarget()
		if target.Target != "helper" || target.Endpoint != "https://studio.example.test:8029" {
			t.Fatalf("target=%+v", target)
		}
		if err := a.SaveFilterRuleToTarget(target.Target, target.Endpoint, &mailfilter.Rule{ID: "one"}, nil); err == nil {
			t.Fatal("remote save must fail without touching local store")
		}
	}
}
func TestFilterTargetRejectsChangedWorkspace(t *testing.T) {
	a := &App{}
	if err := a.SaveFilterRuleToTarget("helper", "https://old.example.test", &mailfilter.Rule{ID: "one"}, nil); err == nil {
		t.Fatal("changed target accepted")
	}
	a.mailboxService = &mailboxServiceConnector{endpoint: "https://new.example.test", statusFunc: func(context.Context) MailboxServiceStatus {
		return MailboxServiceStatus{Connected: true, WriteAuthorized: true}
	}}
	if err := a.SaveFilterRuleToTarget("helper", "https://old.example.test", &mailfilter.Rule{ID: "one"}, nil); err == nil {
		t.Fatal("changed endpoint accepted")
	}
}
func TestFilterTargetLocalServiceUnavailable(t *testing.T) {
	a := &App{mailboxService: &mailboxServiceConnector{statusFunc: func(context.Context) MailboxServiceStatus {
		return MailboxServiceStatus{ErrorCode: "service_unavailable"}
	}}}
	target := a.GetFilterExecutionTarget()
	if target.Target != "local" || !target.Ready {
		t.Fatalf("target=%+v", target)
	}
}

func TestFilterTargetRemoteSaveReadbackAndFailure(t *testing.T) {
	for _, failReadback := range []bool{false, true} {
		t.Run(fmt.Sprint(failReadback), func(t *testing.T) {
			reads, writes := 0, 0
			var saved *mailfilter.Rule
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing grant")
				}
				if r.Method == http.MethodGet {
					reads++
					if reads == 2 && failReadback {
						w.WriteHeader(503)
						return
					}
					rules := []*mailfilter.Rule{}
					if saved != nil {
						rules = append(rules, saved)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": rules})
					return
				}
				writes++
				var req v1.FilterRuleMutationRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				saved = req.Rule
				_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
			}))
			defer server.Close()
			c := automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "test-token"})
			a := &App{mailboxService: &mailboxServiceConnector{endpoint: server.URL, client: c, statusFunc: func(context.Context) MailboxServiceStatus {
				return MailboxServiceStatus{Connected: true, WriteAuthorized: true}
			}}}
			err := a.SaveFilterRuleToTarget("helper", server.URL, &mailfilter.Rule{ID: "one", Name: "Rule", ActionMarkRead: true}, nil)
			if (err != nil) != failReadback {
				t.Fatalf("error=%v", err)
			}
			if reads != 2 || writes != 1 {
				t.Fatalf("reads=%d writes=%d", reads, writes)
			}
		})
	}
}
