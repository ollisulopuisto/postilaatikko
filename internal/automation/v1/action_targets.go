package v1

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hkdb/aerion/internal/operations"
)

// ResolvedActionTarget contains helper-validated account-qualified message refs.
type ResolvedActionTarget struct {
	Messages []operations.MessageRef `json:"messages"`
}

// MountActionTargetRoute validates a caller's explicit target against helper
// state and returns the account-qualified references for a subsequent action.
func MountActionTargetRoute(mux *http.ServeMux, lookup operations.MessageLookup, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/actions/resolve", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		var request struct {
			Target operations.Target `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Malformed target: %v", err), false)
			return
		}
		ids, err := operations.Resolve(request.Target, lookup)
		if err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Target resolution failed: %v", err), false)
			return
		}
		messages, err := lookup.GetByIDs(ids)
		if err != nil || len(messages) != len(ids) {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "One or more target messages are no longer available", false)
			return
		}
		result := ResolvedActionTarget{Messages: make([]operations.MessageRef, 0, len(messages))}
		for _, msg := range messages {
			if msg == nil || msg.ID == "" || msg.AccountID == "" {
				writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Target message has incomplete account identity", false)
				return
			}
			result.Messages = append(result.Messages, operations.MessageRef{AccountID: msg.AccountID, MessageID: msg.ID})
		}
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = "req-unknown"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response[ResolvedActionTarget]{Data: result, RequestID: requestID})
	})
}
