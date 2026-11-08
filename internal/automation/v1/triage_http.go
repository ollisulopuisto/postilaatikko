package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
	Changed bool `json:"changed"`
}

// TriageOperations owns Read Later and Snooze state.
type TriageOperations interface {
	GetReadLaterQueue() ([]triage.Ref, error)
	SetReadLater(accountID, conversationID string, inQueue bool) (bool, error)
	GetSnoozedConversations() ([]triage.Ref, error)
	GetSnoozedCount() (int, error)
	SnoozeConversation(accountID, conversationID string, wakeAt time.Time) error
	UnsnoozeConversation(accountID, conversationID string) (bool, error)
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
	mux.HandleFunc("/v1/triage/read-later/mutations", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeTriageError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		var req ReadLaterMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeTriageError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed Read Later request")
			return
		}
		changed, err := ops.SetReadLater(req.Ref.AccountID, req.Ref.ConversationID, req.InQueue)
		if err != nil {
			writeTriageError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		writeTriageResponse(w, r, TriageMutationResponse{Changed: changed})
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
	mux.HandleFunc("/v1/triage/snoozes/mutations", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeTriageError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		var req SnoozeMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeTriageError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed Snooze request")
			return
		}
		var changed bool
		var err error
		switch req.Operation {
		case "snooze":
			var wake time.Time
			wake, err = time.Parse(time.RFC3339Nano, req.WakeAt)
			if err == nil {
				err = ops.SnoozeConversation(req.Ref.AccountID, req.Ref.ConversationID, wake)
				changed = err == nil
			}
		case "unsnooze":
			changed, err = ops.UnsnoozeConversation(req.Ref.AccountID, req.Ref.ConversationID)
		default:
			err = fmt.Errorf("unknown Snooze operation")
		}
		if err != nil {
			writeTriageError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		writeTriageResponse(w, r, TriageMutationResponse{Changed: changed})
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
