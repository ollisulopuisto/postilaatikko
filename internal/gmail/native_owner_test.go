package gmail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProfileEmailRequiresReturnedIdentity(t *testing.T) {
	for _, email := range []string{"user@example.com", ""} {
		t.Run(email, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/profile" {
					t.Error("unexpected identity request")
				}
				if r.Header.Get("Authorization") != "Bearer account-token" {
					t.Error("identity request used wrong token")
				}
				_, _ = fmt.Fprintf(w, `{"emailAddress":%q}`, email)
			}))
			defer server.Close()
			client := NewClient(func(context.Context, bool) (string, error) { return "account-token", nil })
			client.BaseURL = server.URL
			actual, err := client.ProfileEmail(context.Background())
			if email == "" && err == nil {
				t.Fatal("missing identity accepted")
			}
			if email != "" && (err != nil || actual != email) {
				t.Fatal(actual, err)
			}
		})
	}
}
