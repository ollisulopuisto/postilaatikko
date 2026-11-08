package v1

import (
	"encoding/json"
	"net/http"

	"github.com/hkdb/aerion/internal/views"
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
	CountConversationsVirtualFolder(virtualFolderID string) (int, error)
}

// MountConversationCountRoute mounts the authenticated exact-count operation.
func MountConversationCountRoute(mux *http.ServeMux, ops ConversationCountOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("POST /v1/conversations/count", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		var req ConversationCountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed request body", false)
			return
		}
		if err := req.Scope.Validate(); err != nil || (req.Scope.Kind != views.ScopeFolder && req.Scope.Kind != views.ScopeAllInboxes && req.Scope.Kind != views.ScopeVirtualFolder) {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Invalid conversation count scope", false)
			return
		}
		if req.Scope.Kind == views.ScopeVirtualFolder && req.Filter != "" {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Saved-view counts do not accept a separate filter", false)
			return
		}
		if !validConversationListFilter(req.Filter) {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Unsupported conversation filter", false)
			return
		}
		var count int
		var err error
		if req.Scope.Kind == views.ScopeFolder {
			count, err = ops.CountConversationsByFolder(req.Scope.AccountID, req.Scope.FolderID, req.Filter)
		} else if req.Scope.Kind == views.ScopeAllInboxes {
			count, err = ops.CountConversationsUnifiedInbox(req.Filter)
		} else {
			count, err = ops.CountConversationsVirtualFolder(req.Scope.VirtualFolderID)
		}
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to count conversations", true)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, count)
	})
}
