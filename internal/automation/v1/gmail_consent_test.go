package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type gmailConsentStub struct {
	err    error
	calls  int
	client string
}

func (s *gmailConsentStub) GmailConsent(_ context.Context, client string, _ GmailConsentRequest) (GmailConsentReply, error) {
	s.calls++
	s.client = client
	return GmailConsentReply{Authorized: true}, s.err
}
func TestHelperConsentRouteAuthorizationAndNoCredentialAcceptance(t *testing.T) {
	for _, manage := range []bool{false, true} {
		stub := &gmailConsentStub{}
		mux := http.NewServeMux()
		MountGmailConsentRoutes(mux, stub, func(*http.Request) string { return "authenticated-client" }, func(w http.ResponseWriter, r *http.Request, c Capability) bool {
			if c == CapabilityAccountManage && !manage {
				w.WriteHeader(403)
				return false
			}
			return true
		})
		for _, op := range []string{"start", "complete", "cancel"} {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail/consent", strings.NewReader(`{"operation":"`+op+`","accountId":"helper"}`)))
			if !manage {
				if rec.Code != 403 || stub.calls != 0 {
					t.Fatal("read grant reached consent")
				}
				continue
			}
			if rec.Code != 200 || stub.client != "authenticated-client" {
				t.Fatal(rec.Code, stub.client)
			}
		}
		if !manage {
			continue
		}
		for _, body := range []string{`{"operation":"start","clientId":"spoof"}`, `{"operation":"complete","refreshToken":"injected"}`, `{"operation":"complete","accessToken":"injected"}`, `{"operation":"unknown"}`, `{} {}`, strings.Repeat("x", 17000)} {
			before := stub.calls
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail/consent", strings.NewReader(body)))
			if rec.Code != 400 || stub.calls != before {
				t.Fatal("malformed/credential request reached helper", rec.Code)
			}
		}
	}
}

func TestGmailConsentStubDoesNotExposeBackendErrors(t *testing.T) {
	const secret = "synthetic-private-keyring-detail"
	mux := http.NewServeMux()
	MountGmailConsentRoutes(mux, &gmailConsentStub{err: errors.New(secret)}, func(*http.Request) string { return "client" }, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail/consent", strings.NewReader(`{"operation":"start","accountId":"helper"}`)))
	var envelope ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != CodeConflict || rec.Code != 409 || strings.Contains(rec.Body.String(), secret) || !strings.Contains(rec.Body.String(), "select the Gmail account") || !strings.Contains(rec.Body.String(), "Gmail settings permission") {
		t.Fatal("backend error disclosure", rec.Code, rec.Body.String())
	}
}

func TestGmailConsentRejectsMissingAccountBeforeOwner(t *testing.T) {
	stub := &gmailConsentStub{}
	mux := http.NewServeMux()
	MountGmailConsentRoutes(mux, stub, func(*http.Request) string { return "client" }, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	for _, body := range []string{`{"operation":"start"}`, `{"operation":"complete","accountId":" "}`} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail/consent", strings.NewReader(body)))
		var envelope ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Error.Code != CodeInvalidRequest || rec.Code != 400 || stub.calls != 0 {
			t.Fatal("missing account reached owner", rec.Code, stub.calls)
		}
	}
}
