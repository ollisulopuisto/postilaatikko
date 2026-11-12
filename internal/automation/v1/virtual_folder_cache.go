package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/hkdb/aerion/internal/database"
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

// cachedViewRouteDeadline bounds how long the cache route waits for the
// store before answering database_busy. Desktop callers time out after two
// seconds (mailboxAPIRequestTimeout), so the route must answer within that
// budget; the store operation keeps running in the background and its result
// (an idempotent cache refresh) is not lost when the client disconnects.
var cachedViewRouteDeadline = 1500 * time.Millisecond

type cachedViewResult struct {
	view *CachedVirtualFolderView
	err  error
}

// MountVirtualFolderCacheRoute mounts the authenticated cached-result read.
func MountVirtualFolderCacheRoute(mux *http.ServeMux, ops CachedVirtualFolderOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/views/{virtualFolderID}/cache", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		done := make(chan cachedViewResult, 1)
		go func() {
			view, err := ops.GetCachedVirtualFolderView(r.PathValue("virtualFolderID"))
			done <- cachedViewResult{view, err}
		}()
		var result cachedViewResult
		select {
		case result = <-done:
		case <-time.After(cachedViewRouteDeadline):
			writeHeaderError(w, r, http.StatusServiceUnavailable, CodeDatabaseBusy, "Mailbox database is busy", true)
			return
		}
		err := result.err
		if errors.Is(err, ErrCachedVirtualFolderNotFound) {
			writeHeaderError(w, r, http.StatusNotFound, CodeNotFound, "Virtual folder not found", false)
			return
		}
		if err != nil && database.IsBusy(err) {
			// Transient lock contention gets a distinct retryable code; the
			// raw SQLite error never leaves the process.
			writeHeaderError(w, r, http.StatusServiceUnavailable, CodeDatabaseBusy, "Mailbox database is busy", true)
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
		_ = json.NewEncoder(w).Encode(Response[*CachedVirtualFolderView]{Data: result.view, RequestID: requestID})
	})
}
