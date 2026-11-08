package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/hkdb/aerion/internal/message"
)

var ErrCachedVirtualFolderNotFound = errors.New("virtual folder not found")

// CachedVirtualFolderView returns a bounded server-cached membership index
// hydrated with current header and flag state.
type CachedVirtualFolderView struct {
	VirtualFolderID string                  `json:"virtualFolderId"`
	Conversations   []*message.Conversation `json:"conversations"`
	RefreshedAt     time.Time               `json:"refreshedAt"`
	HasCache        bool                    `json:"hasCache"`
}

type CachedVirtualFolderOperations interface {
	GetCachedVirtualFolderView(string) (*CachedVirtualFolderView, error)
}

// MountVirtualFolderCacheRoute mounts the authenticated cached-result read.
func MountVirtualFolderCacheRoute(mux *http.ServeMux, ops CachedVirtualFolderOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/views/{virtualFolderID}/cache", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		view, err := ops.GetCachedVirtualFolderView(r.PathValue("virtualFolderID"))
		if errors.Is(err, ErrCachedVirtualFolderNotFound) {
			writeHeaderError(w, r, http.StatusNotFound, CodeNotFound, "Virtual folder not found", false)
			return
		}
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not load virtual-folder cache", true)
			return
		}
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = "req-unknown"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response[*CachedVirtualFolderView]{Data: view, RequestID: requestID})
	})
}
