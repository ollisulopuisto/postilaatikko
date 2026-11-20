package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type gmailConsentStub struct {
	calls  int
	client string
}

func (s *gmailConsentStub) GmailConsent(_ context.Context, client string, _ GmailConsentRequest) (GmailConsentReply, error) {
	s.calls++
	s.client = client
	return GmailConsentReply{Authorized: true}, nil
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
