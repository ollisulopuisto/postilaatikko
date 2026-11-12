package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
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
	GetCachedVirtualFolderView(context.Context, string) (*CachedVirtualFolderView, error)
}

// cachedViewRouteDeadline bounds how long the cache route waits for the
// store before answering cache_timeout. Desktop callers time out after two
// seconds (mailboxAPIRequestTimeout), so the route must answer within that
// budget; the store operation keeps running in the background and its result
// (an idempotent cache refresh) is not lost when the client disconnects.
var cachedViewRouteDeadline = 1500 * time.Millisecond

// detachedCacheWorkBudget bounds the background refresh after the route has
// answered. Without it an abandoned refresh could wait on the write lock for
// the full retry budget (five 30-second attempts), and repeated requests
// would occupy the helper's bounded database pool long after their clients
// left.
var detachedCacheWorkBudget = 10 * time.Second

type cachedViewResult struct {
	view *CachedVirtualFolderView
	err  error
}

// cachedViewFlight is one in-flight cache refresh per view. Concurrent
// requests join the flight instead of each starting their own goroutine, so
// a burst of timed-out requests cannot pile abandoned cache writes onto the
// database pool.
type cachedViewFlight struct {
	done   chan struct{}
	result cachedViewResult
}

var cachedViewFlights sync.Map // virtualFolderID -> *cachedViewFlight

// MountVirtualFolderCacheRoute mounts the authenticated cached-result read.
func MountVirtualFolderCacheRoute(mux *http.ServeMux, ops CachedVirtualFolderOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/views/{virtualFolderID}/cache", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		virtualFolderID := r.PathValue("virtualFolderID")
		flight := &cachedViewFlight{done: make(chan struct{})}
		existing, loaded := cachedViewFlights.LoadOrStore(virtualFolderID, flight)
		if loaded {
			flight = existing.(*cachedViewFlight)
		} else {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), detachedCacheWorkBudget)
				defer cancel()
				view, err := ops.GetCachedVirtualFolderView(ctx, virtualFolderID)
				flight.result = cachedViewResult{view, err}
				close(flight.done)
				cachedViewFlights.Delete(virtualFolderID)
			}()
		}
		var result cachedViewResult
		select {
		case <-flight.done:
			result = flight.result
		case <-time.After(cachedViewRouteDeadline):
			// The refresh is still running, but nothing says the database is
			// locked: healthy hydration of a large cached view can exceed the
			// deadline on its own. Label it cache_timeout so database_busy
			// stays truthful for real lock contention.
			writeHeaderError(w, r, http.StatusServiceUnavailable, CodeCacheTimeout, "Virtual-folder cache refresh exceeded its time budget", true)
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
