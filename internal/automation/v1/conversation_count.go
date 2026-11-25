package v1

import (
	"encoding/json"
	"net/http"

	"github.com/ollisulopuisto/postilaatikko/internal/views"
)

// ConversationCountRequest asks the helper for a filtered conversation count.
type ConversationCountRequest struct {
	Scope  views.Scope `json:"scope"`
	Filter string      `json:"filter,omitempty"`
}

// ConversationCountOperations counts conversations within supported scopes.
type ConversationCountOperations interface {
	CountConversationsByFolder(accountID, folderID, filter string) (int, error)
	CountConversationsUnifiedInbox(filter string) (int, error)
	CountConversationsUnifiedFolder(folderType, filter string) (int, error)
	CountConversationsVirtualFolder(virtualFolderID string) (int, error)
}

// MountConversationCountRoute mounts the authenticated exact-count operation.
func MountConversationCountRoute(mux RouteMux, ops ConversationCountOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("POST /v1/conversations/count", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		var req ConversationCountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed request body", false)
			return
		}
		count, reqErr := executeConversationCount(ops, req)
		if reqErr != nil {
			writeHeaderError(w, r, reqErr.status, reqErr.code, reqErr.message, reqErr.retryable)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, count)
	})
}

// ExecuteConversationCount is the single implementation of an exact
// conversation count, shared by POST /v1/conversations/count and the
// desktop's in-process local mode.
func ExecuteConversationCount(ops ConversationCountOperations, req ConversationCountRequest) (int, error) {
	count, reqErr := executeConversationCount(ops, req)
	if reqErr != nil {
		return 0, reqErr.public()
	}
	return count, nil
}

func executeConversationCount(ops ConversationCountOperations, req ConversationCountRequest) (int, *requestError) {
	if err := req.Scope.Validate(); err != nil || (req.Scope.Kind != views.ScopeFolder && req.Scope.Kind != views.ScopeAllInboxes && req.Scope.Kind != views.ScopeUnifiedFolder && req.Scope.Kind != views.ScopeVirtualFolder) {
		return 0, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Invalid conversation count scope", false}
	}
	if req.Scope.Kind == views.ScopeVirtualFolder && req.Filter != "" {
		return 0, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Saved-view counts do not accept a separate filter", false}
	}
	if !validConversationListFilter(req.Filter) {
		return 0, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Unsupported conversation filter", false}
	}
	var count int
	var err error
	switch req.Scope.Kind {
	case views.ScopeFolder:
		count, err = ops.CountConversationsByFolder(req.Scope.AccountID, req.Scope.FolderID, req.Filter)
	case views.ScopeAllInboxes:
		count, err = ops.CountConversationsUnifiedInbox(req.Filter)
	case views.ScopeUnifiedFolder:
		count, err = ops.CountConversationsUnifiedFolder(req.Scope.FolderType, req.Filter)
	default:
		count, err = ops.CountConversationsVirtualFolder(req.Scope.VirtualFolderID)
	}
	if err != nil {
		return 0, &requestError{http.StatusInternalServerError, CodeInternal, "Failed to count conversations", true}
	}
	return count, nil
}
