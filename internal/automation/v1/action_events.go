package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ActionEvent is a content-free mailbox mutation notice for cache reconciliation.
type ActionEvent struct {
	Sequence       int64                `json:"sequence"`
	OperationID    string               `json:"operationId"`
	State          string               `json:"state"`
	Affected       []AffectedMessageRef `json:"affected,omitempty"`
	AffectedTriage []AffectedTriageRef  `json:"affectedTriage,omitempty"`
	Partial        bool                 `json:"partial,omitempty"`
	Groups         []ActionGroupOutcome `json:"groups,omitempty"`
	UndoAvailable  bool                 `json:"undoAvailable,omitempty"`
	CreatedAt      string               `json:"createdAt"`
}

// ActionEventPage is one bounded page after the authenticated client's cursor.
type ActionEventPage struct {
	Events        []ActionEvent `json:"events"`
	Cursor        int64         `json:"cursor"`
	ResetRequired bool          `json:"resetRequired,omitempty"`
}

// ActionEventStore persists events and acknowledgments scoped to an API client.
type ActionEventStore interface {
	ListActionEvents(context.Context, string, int) (ActionEventPage, error)
	AcknowledgeActionEvents(context.Context, string, int64) error
}

// MountActionEventRoutes mounts bounded action-event recovery routes.
func MountActionEventRoutes(mux *http.ServeMux, store ActionEventStore, clientID func(*http.Request) string, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/action-events", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		id := clientID(r)
		if strings.TrimSpace(id) == "" {
			writeHeaderError(w, r, http.StatusUnauthorized, CodeUnauthenticated, "Authenticated client identity is unavailable", false)
			return
		}
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "limit must be between 1 and 100", false)
				return
			}
			limit = parsed
		}
		page, err := store.ListActionEvents(r.Context(), id, limit)
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not load action events", true)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, page)
	})
	mux.HandleFunc("/v1/action-events/ack", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		id := clientID(r)
		if strings.TrimSpace(id) == "" {
			writeHeaderError(w, r, http.StatusUnauthorized, CodeUnauthenticated, "Authenticated client identity is unavailable", false)
			return
		}
		var request struct {
			Sequence int64 `json:"sequence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Sequence < 0 {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "sequence must be a non-negative integer", false)
			return
		}
		if err := store.AcknowledgeActionEvents(r.Context(), id, request.Sequence); err != nil {
			writeHeaderError(w, r, http.StatusConflict, CodeInvalidRequest, "Could not acknowledge action-event cursor", false)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, map[string]int64{"cursor": request.Sequence})
	})
}
