package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

// ConversationSearchRequest asks the helper's indexed header cache for one
// bounded page of results within a unified system folder.
type ConversationSearchRequest struct {
	Scope  views.Scope `json:"scope"`
	Query  string      `json:"query"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
	Filter string      `json:"filter,omitempty"`
}

// ConversationSearchPage contains indexed header results and their exact count.
type ConversationSearchPage struct {
	Results    []*message.ConversationSearchResult `json:"results"`
	TotalCount int                                 `json:"totalCount"`
}

// ConversationSearchOperations searches a helper-owned unified system folder.
type ConversationSearchOperations interface {
	SearchConversationsUnifiedFolder(folderType, query string, offset, limit int, filter string) ([]*message.ConversationSearchResult, int, error)
}

// MountConversationSearchRoute mounts bounded unified-folder header search.
func MountConversationSearchRoute(mux *http.ServeMux, ops ConversationSearchOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("POST /v1/conversations/search", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		var request ConversationSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed request body", false)
			return
		}
		if err := request.Scope.Validate(); err != nil || request.Scope.Kind != views.ScopeUnifiedFolder {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Search requires a supported unified-folder scope", false)
			return
		}
		request.Query = strings.TrimSpace(request.Query)
		if request.Query == "" || len(request.Query) > 512 {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "query must contain between 1 and 512 bytes", false)
			return
		}
		if request.Offset < 0 || request.Offset > MaximumMessageOffset {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "offset is invalid or exceeds the maximum offset", false)
			return
		}
		if request.Limit < 1 || request.Limit > MaximumPageSize {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "limit is invalid", false)
			return
		}
		if !validConversationListFilter(request.Filter) {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Unsupported conversation filter", false)
			return
		}
		results, count, err := ops.SearchConversationsUnifiedFolder(request.Scope.FolderType, request.Query, request.Offset, request.Limit, request.Filter)
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to search conversations", true)
			return
		}
		if results == nil {
			results = []*message.ConversationSearchResult{}
		}
		writeAPIResponse(w, r, http.StatusOK, ConversationSearchPage{Results: results, TotalCount: count})
	})
}
