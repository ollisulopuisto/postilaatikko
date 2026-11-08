package v1

import (
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
	Changed       bool             `json:"changed"`
	Receipt       OperationReceipt `json:"receipt"`
	UndoAvailable bool             `json:"undoAvailable,omitempty"`
	UndoExpiresAt string           `json:"undoExpiresAt,omitempty"`
}

// ReplyLaterHTTPOperations owns Reply Later queue reads and changes.
type ReplyLaterHTTPOperations interface {
	GetReplyLaterQueue() ([]replylater.Ref, error)
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
