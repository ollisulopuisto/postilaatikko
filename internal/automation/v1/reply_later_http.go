package v1

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/hkdb/aerion/internal/replylater"
)

// ReplyLaterMutationRequest sets explicit queue membership for one conversation.
type ReplyLaterMutationRequest struct {
	Ref     replylater.Ref `json:"ref"`
	InQueue bool           `json:"inQueue"`
}

// ReplyLaterMutationResponse reports whether queue membership changed.
type ReplyLaterMutationResponse struct {
	Changed bool `json:"changed"`
}

// ReplyLaterHTTPOperations owns Reply Later queue reads and changes.
type ReplyLaterHTTPOperations interface {
	GetReplyLaterQueue() ([]replylater.Ref, error)
	SetReplyLaterMembership(context.Context, string, string, bool) (bool, error)
}

// MountReplyLaterRoutes mounts authenticated Reply Later queue operations.
func MountReplyLaterRoutes(mux *http.ServeMux, ops ReplyLaterHTTPOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/reply-later", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeReplyLaterError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		queue, err := ops.GetReplyLaterQueue()
		if err != nil {
			writeReplyLaterError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to load Reply Later queue")
			return
		}
		writeReplyLaterResponse(w, r, queue)
	})
	mux.HandleFunc("/v1/reply-later/mutations", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeReplyLaterError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		var req ReplyLaterMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeReplyLaterError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed Reply Later request")
			return
		}
		changed, err := ops.SetReplyLaterMembership(r.Context(), req.Ref.AccountID, req.Ref.ConversationID, req.InQueue)
		if err != nil {
			writeReplyLaterError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		writeReplyLaterResponse(w, r, ReplyLaterMutationResponse{Changed: changed})
	})
}

func writeReplyLaterResponse(w http.ResponseWriter, r *http.Request, data any) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(Response[any]{Data: data, RequestID: reqID})
}

func writeReplyLaterError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, message string) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: ErrorBody{Code: code, Message: message, RequestID: reqID}})
}
