package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type gmailPublicationStub struct {
	calls int
	err   error
}

func (s *gmailPublicationStub) GmailPublication(context.Context, GmailPublicationRequest) (GmailPublicationReply, error) {
	s.calls++
	return GmailPublicationReply{AccountID: "helper"}, s.err
}
func TestGmailPublicationRouteRequiresBothWriteAndAccountAuthority(t *testing.T) {
	for _, op := range []string{"context", "preview", "status", "reconcile", "prepare", "publish", "cancel"} {
		for _, mailWrite := range []bool{false, true} {
			for _, accountManage := range []bool{false, true} {
				t.Run(op+string(rune('0'+boolNumber(mailWrite)))+string(rune('0'+boolNumber(accountManage))), func(t *testing.T) {
					stub := &gmailPublicationStub{}
					mux := http.NewServeMux()
					MountGmailPublicationRoutes(mux, stub, func(w http.ResponseWriter, r *http.Request, c Capability) bool {
						allowed := c == CapabilityMailRead || (c == CapabilityMailWrite && mailWrite) || (c == CapabilityAccountManage && accountManage)
						if !allowed {
							w.WriteHeader(403)
						}
						return allowed
					})
					rec := httptest.NewRecorder()
					mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail", strings.NewReader(`{"operation":"`+op+`","accountId":"helper"}`)))
					mutates := op == "prepare" || op == "publish" || op == "cancel" || op == "reconcile"
					if mutates && (!mailWrite || !accountManage) {
						if rec.Code != 403 || stub.calls != 0 {
							t.Fatal("insufficient grant reached owner", rec.Code, stub.calls)
						}
					} else if rec.Code != 200 || stub.calls != 1 {
						t.Fatal(rec.Code, stub.calls)
					}
				})
			}
		}
	}
}
func boolNumber(value bool) int {
	if value {
		return 1
	}
	return 0
}
func TestGmailPublicationRouteBoundsAndRejectsCredentialFields(t *testing.T) {
	stub := &gmailPublicationStub{}
	mux := http.NewServeMux()
	MountGmailPublicationRoutes(mux, stub, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	for _, body := range []string{`{"operation":"publish","refreshToken":"injected"}`, `{"operation":"unknown"}`, `{"operation":"preview"} {}`, strings.Repeat("x", 33000)} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail", strings.NewReader(body)))
		if rec.Code != 400 || stub.calls != 0 {
			t.Fatal("invalid request reached helper", rec.Code, stub.calls)
		}
	}
}

func TestGmailPublicationStubDoesNotExposeBackendErrors(t *testing.T) {
	const secret = "synthetic-private-keyring-detail"
	mux := http.NewServeMux()
	MountGmailPublicationRoutes(mux, &gmailPublicationStub{err: errors.New(secret)}, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail", strings.NewReader(`{"operation":"status","accountId":"helper"}`)))
	if rec.Code != 409 || strings.Contains(rec.Body.String(), secret) {
		t.Fatal("backend error disclosure", rec.Code, rec.Body.String())
	}
}

func TestGmailPublicationEligibilityErrorIsSafeAndNotRetryAdvice(t *testing.T) {
	mux := http.NewServeMux()
	MountGmailPublicationRoutes(mux, &gmailPublicationStub{err: ErrGmailAccountIneligible}, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail", strings.NewReader(`{"operation":"status","ruleId":"outlook-rule"}`)))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose a Gmail account") || strings.Contains(rec.Body.String(), "reload") {
		t.Fatal("ineligible account got recovery advice", rec.Code, rec.Body.String())
	}
}
