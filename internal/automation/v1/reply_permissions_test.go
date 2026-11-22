package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReplyPreparationRequiresEveryDisclosureGrant(t *testing.T) {
	for _, missing := range []Capability{CapabilityMailRead, CapabilityAttachmentRead, CapabilityMailBodyFetch, CapabilityDraftWrite} {
		t.Run(string(missing), func(t *testing.T) {
			mux := http.NewServeMux()
			NewDraftsHandler(nil, nil).MountRoutes(mux, func(w http.ResponseWriter, r *http.Request, cap Capability) bool {
				if cap == missing {
					w.WriteHeader(http.StatusForbidden)
					return false
				}
				return true
			})
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/replies", strings.NewReader(`{"accountId":"a","messageId":"live-with-retained-content","mode":"forward"}`)))
			if rec.Code != http.StatusForbidden {
				t.Fatal("reply returned retained content without grant", missing, rec.Code)
			}
		})
	}
}
