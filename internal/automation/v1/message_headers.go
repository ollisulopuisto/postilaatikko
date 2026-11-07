package v1

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hkdb/aerion/internal/message"
)

const maximumMessageOffset = 1_000_000

// MessageHeaderPage is a bounded page of cached message headers.
type MessageHeaderPage struct {
	Items      []*message.MessageHeader `json:"items"`
	NextCursor string                   `json:"nextCursor,omitempty"`
	Complete   bool                     `json:"complete"`
}

// MountMessageHeaderRoutes mounts the account-qualified header list route.
// The enclosing automation server must authenticate the request.
func MountMessageHeaderRoutes(mux *http.ServeMux, ops MessageHeaderOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/messages/headers", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodGet {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		handleMessageHeaders(w, r, ops)
	})
}

func handleMessageHeaders(w http.ResponseWriter, r *http.Request, ops MessageHeaderOperations) {
	accountID := r.URL.Query().Get("accountId")
	folderID := r.URL.Query().Get("folderId")
	if accountID == "" || folderID == "" {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "accountId and folderId are required", false)
		return
	}

	pageRequest := PageRequest{Limit: DefaultPageSize}
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "limit must be an integer", false)
			return
		}
		pageRequest.Limit = limit
	}
	pageRequest, err := pageRequest.Normalized()
	if err != nil {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error(), false)
		return
	}

	offset := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 || offset > maximumMessageOffset {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "cursor is invalid or exceeds the maximum offset", false)
			return
		}
	}

	f, err := ops.GetFolder(folderID)
	if err != nil {
		writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to load folder", true)
		return
	}
	if f == nil || f.AccountID != accountID {
		writeHeaderError(w, r, http.StatusNotFound, CodeNotFound, "Folder not found for account", false)
		return
	}

	items, err := ops.ListByFolder(folderID, offset, pageRequest.Limit+1)
	if err != nil {
		writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Failed to load message headers", true)
		return
	}
	page := MessageHeaderPage{Items: items, Complete: true}
	if len(items) > pageRequest.Limit {
		page.Items = items[:pageRequest.Limit]
		page.Complete = false
		page.NextCursor = strconv.Itoa(offset + pageRequest.Limit)
	}
	if page.Items == nil {
		page.Items = []*message.MessageHeader{}
	}
	writeHeaderResponse(w, r, http.StatusOK, page)
}

func writeHeaderResponse(w http.ResponseWriter, r *http.Request, status int, data MessageHeaderPage) {
	writeAPIResponse(w, r, status, data)
}

func writeAPIResponse[T any](w http.ResponseWriter, r *http.Request, status int, data T) {
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response[T]{Data: data, RequestID: requestID})
}

func writeHeaderError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, message string, retryable bool) {
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: ErrorBody{
		Code: code, Message: message, Retryable: retryable, RequestID: requestID,
	}})
}
