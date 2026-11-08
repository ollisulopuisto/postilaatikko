package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"context"
)

// FolderSyncRequest asks the helper to refresh one account's provider folders.
type FolderSyncRequest struct {
	AccountID string `json:"accountId"`
}

// FolderSubscriptionRequest sets one account-qualified folder's subscription state.
type FolderSubscriptionRequest struct {
	AccountID  string `json:"accountId"`
	FolderID   string `json:"folderId"`
	Subscribed *bool  `json:"subscribed"`
}

// FolderSubscriptionsRequest changes subscriptions for all folders in an account.
type FolderSubscriptionsRequest struct {
	AccountID  string `json:"accountId"`
	Subscribed *bool  `json:"subscribed"`
}

// FolderMutationOperations owns provider folder discovery and subscriptions.
type FolderMutationOperations interface {
	SyncFolders(context.Context, string) error
	SetFolderSubscription(context.Context, string, string, bool) error
	SetAllFolderSubscriptions(context.Context, string, bool) error
}

// MountFolderMutationRoutes mounts authenticated provider-folder operations.
func MountFolderMutationRoutes(mux *http.ServeMux, ops FolderMutationOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/folders/sync", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		var req FolderSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AccountID) == "" {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid accountId is required", false)
			return
		}
		if err := ops.SyncFolders(r.Context(), req.AccountID); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error(), false)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, struct{}{})
	})
	mux.HandleFunc("/v1/folders/subscription", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		var req FolderSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AccountID) == "" || strings.TrimSpace(req.FolderID) == "" || req.Subscribed == nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid accountId, folderId, and subscribed value are required", false)
			return
		}
		if err := ops.SetFolderSubscription(r.Context(), req.AccountID, req.FolderID, *req.Subscribed); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error(), false)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, struct{}{})
	})
	mux.HandleFunc("/v1/folders/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		var req FolderSubscriptionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AccountID) == "" || req.Subscribed == nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid accountId and subscribed value are required", false)
			return
		}
		if err := ops.SetAllFolderSubscriptions(r.Context(), req.AccountID, *req.Subscribed); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error(), false)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, struct{}{})
	})
}
