package v1

import (
	"encoding/json"
	"net/http"

	"github.com/hkdb/aerion/internal/triage"
)

// ReadLaterMutationRequest sets explicit Read Later membership.
type ReadLaterMutationRequest struct {
	Ref     triage.Ref `json:"ref"`
	InQueue bool       `json:"inQueue"`
}

// SnoozeMutationRequest changes or removes one snooze deadline.
type SnoozeMutationRequest struct {
	Operation string     `json:"operation"`
	Ref       triage.Ref `json:"ref"`
	WakeAt    string     `json:"wakeAt,omitempty"`
}

// TriageMutationResponse reports whether the requested membership changed.
type TriageMutationResponse struct {
	Changed        bool                `json:"changed"`
	Receipt        OperationReceipt    `json:"receipt"`
	AffectedTriage []AffectedTriageRef `json:"affectedTriage,omitempty"`
	UndoAvailable  bool                `json:"undoAvailable,omitempty"`
	UndoExpiresAt  string              `json:"undoExpiresAt,omitempty"`
}

// TriageOperations owns Read Later and Snooze state.
type TriageOperations interface {
	GetReadLaterQueue() ([]triage.Ref, error)
	GetSnoozedConversations() ([]triage.Ref, error)
	GetSnoozedCount() (int, error)
}

// MountTriageRoutes mounts authenticated Read Later and Snooze operations.
func MountTriageRoutes(mux *http.ServeMux, ops TriageOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/triage/read-later", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeTriageError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		items, err := ops.GetReadLaterQueue()
		if err != nil {
			writeTriageError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to load Read Later queue")
			return
		}
		writeTriageResponse(w, r, items)
	})
	mux.HandleFunc("/v1/triage/snoozes", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeTriageError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		items, err := ops.GetSnoozedConversations()
		if err != nil {
			writeTriageError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to load Snoozed conversations")
			return
		}
		writeTriageResponse(w, r, items)
	})
	mux.HandleFunc("/v1/triage/snoozes/count", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeTriageError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		count, err := ops.GetSnoozedCount()
		if err != nil {
			writeTriageError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to count Snoozed conversations")
			return
		}
		writeTriageResponse(w, r, count)
	})
}

func writeTriageResponse(w http.ResponseWriter, r *http.Request, data any) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(Response[any]{Data: data, RequestID: reqID})
}

func writeTriageError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, message string) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: ErrorBody{Code: code, Message: message, RequestID: reqID}})
}
