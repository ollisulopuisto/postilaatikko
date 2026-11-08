package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hkdb/aerion/internal/draft"
	"github.com/hkdb/aerion/internal/smtp"
)

var ErrDraftRevisionConflict = draft.ErrDraftRevisionConflict

// DraftDocument is the compose-facing view of a helper-owned draft.
type DraftDocument struct {
	ID               string              `json:"id"`
	AccountID        string              `json:"accountId"`
	Revision         string              `json:"revision"`
	UpdatedAt        string              `json:"updatedAt"`
	Message          smtp.ComposeMessage `json:"message"`
	Encrypted        bool                `json:"encrypted,omitempty"`
	EncryptedBody    []byte              `json:"encryptedBody,omitempty"`
	PGPEncrypted     bool                `json:"pgpEncrypted,omitempty"`
	PGPEncryptedBody []byte              `json:"pgpEncryptedBody,omitempty"`
	AttachmentsData  []byte              `json:"attachmentsData,omitempty"`
}

// DraftMutation creates or replaces a draft. Revision is mandatory for updates.
type DraftMutation struct {
	ID               string              `json:"id,omitempty"`
	AccountID        string              `json:"accountId"`
	Revision         string              `json:"revision,omitempty"`
	Message          smtp.ComposeMessage `json:"message"`
	Encrypted        bool                `json:"encrypted,omitempty"`
	EncryptedBody    []byte              `json:"encryptedBody,omitempty"`
	PGPEncrypted     bool                `json:"pgpEncrypted,omitempty"`
	PGPEncryptedBody []byte              `json:"pgpEncryptedBody,omitempty"`
	AttachmentsData  []byte              `json:"attachmentsData,omitempty"`
}

type ReplyPreparationRequest struct {
	AccountID string `json:"accountId"`
	MessageID string `json:"messageId"`
	Mode      string `json:"mode"`
}

type SendRequest struct {
	AccountID string   `json:"accountId"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	RFC822    []byte   `json:"rfc822"`
	DraftID   string   `json:"draftId,omitempty"`
	Revision  string   `json:"revision,omitempty"`
}

type SendResponse struct {
	Receipt OperationReceipt `json:"receipt"`
	Code    string           `json:"code,omitempty"`
}

var ErrSendIdempotencyConflict = errors.New("send idempotency key conflict")

// DraftOperations persists account-scoped compose state and delegates delivery.
type DraftOperations interface {
	ListDrafts(string) ([]DraftDocument, error)
	GetDraft(string, string) (*DraftDocument, error)
	SaveDraft(string, DraftMutation) (*DraftDocument, error)
	DeleteDraft(string, string, string) error
	PrepareReply(ReplyPreparationRequest) (*smtp.ComposeMessage, error)
	Send(context.Context, SendRequest, string, string) (*SendResponse, error)
}

// DraftsHandler mounts account-qualified draft, reply-preparation and send routes.
type DraftsHandler struct {
	operations  DraftOperations
	fingerprint func([]byte) []byte
}

func NewDraftsHandler(operations DraftOperations, fingerprint func([]byte) []byte) *DraftsHandler {
	return &DraftsHandler{operations: operations, fingerprint: fingerprint}
}

func (h *DraftsHandler) MountRoutes(mux *http.ServeMux, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftRead) {
			return
		}
		accountID := strings.TrimSpace(r.URL.Query().Get("accountId"))
		if accountID == "" {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "accountId is required", false)
			return
		}
		items, err := h.operations.ListDrafts(accountID)
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not list drafts", true)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, items)
	})
	mux.HandleFunc("GET /v1/drafts/{draftID}", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftRead) {
			return
		}
		item, err := h.operations.GetDraft(r.PathValue("draftID"), r.URL.Query().Get("accountId"))
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read draft", true)
			return
		}
		if item == nil {
			writeHeaderError(w, r, http.StatusNotFound, CodeNotFound, "Draft not found", false)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, item)
	})
	mux.HandleFunc("POST /v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftWrite) {
			return
		}
		var req DraftMutation
		if !decodeDraftRequest(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.AccountID) == "" || req.Revision != "" || len(req.ID) > 128 {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "accountId is required and a new draft cannot include a revision", false)
			return
		}
		item, err := h.operations.SaveDraft("", req)
		if err != nil {
			writeDraftError(w, r, err)
			return
		}
		writeAPIResponse(w, r, http.StatusCreated, item)
	})
	mux.HandleFunc("PUT /v1/drafts/{draftID}", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftWrite) {
			return
		}
		var req DraftMutation
		if !decodeDraftRequest(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.AccountID) == "" || strings.TrimSpace(req.Revision) == "" || req.ID != "" && req.ID != r.PathValue("draftID") {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "accountId and revision are required", false)
			return
		}
		item, err := h.operations.SaveDraft(r.PathValue("draftID"), req)
		if err != nil {
			writeDraftError(w, r, err)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, item)
	})
	mux.HandleFunc("DELETE /v1/drafts/{draftID}", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftWrite) {
			return
		}
		accountID, revision := r.URL.Query().Get("accountId"), r.Header.Get("If-Match")
		revision = strings.Trim(revision, `"`)
		if accountID == "" || revision == "" {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "accountId and If-Match revision are required", false)
			return
		}
		if err := h.operations.DeleteDraft(r.PathValue("draftID"), accountID, revision); err != nil {
			writeDraftError(w, r, err)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, map[string]bool{"deleted": true})
	})
	mux.HandleFunc("POST /v1/replies", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityDraftWrite) {
			return
		}
		var req ReplyPreparationRequest
		if !decodeDraftRequest(w, r, &req) {
			return
		}
		if req.AccountID == "" || req.MessageID == "" || (req.Mode != "reply" && req.Mode != "reply-all" && req.Mode != "forward") {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "valid accountId, messageId, and mode are required", false)
			return
		}
		message, err := h.operations.PrepareReply(req)
		if err != nil {
			writeDraftError(w, r, err)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, message)
	})
	mux.HandleFunc("POST /v1/send", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailSend) {
			return
		}
		var req SendRequest
		if !decodeDraftRequest(w, r, &req) {
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" || len(key) > 200 {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "A valid Idempotency-Key header is required", false)
			return
		}
		if h.fingerprint == nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Secure send fingerprinting is unavailable", false)
			return
		}
		requestBytes, err := json.Marshal(struct {
			Method string      `json:"method"`
			Path   string      `json:"path"`
			Body   SendRequest `json:"body"`
		}{r.Method, r.URL.Path, req})
		if err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed send request", false)
			return
		}
		defer clear(requestBytes)
		fingerprint := h.fingerprint(requestBytes)
		clear(requestBytes)
		if len(fingerprint) == 0 {
			clear(req.RFC822)
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Secure send fingerprinting failed", false)
			return
		}
		response, err := h.operations.Send(r.Context(), req, key, fmt.Sprintf("%x", fingerprint))
		clear(req.RFC822)
		if err != nil {
			writeDraftError(w, r, err)
			return
		}
		writeAPIResponse(w, r, http.StatusOK, response)
	})
}

func decodeDraftRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 28<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed or unsupported draft request", false)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Request must contain one JSON value", false)
		return false
	}
	return true
}

func writeDraftError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrSendIdempotencyConflict) {
		writeHeaderError(w, r, http.StatusConflict, CodeIdempotencyConflict, "Idempotency-Key was already used for a different send", false)
		return
	}
	if errors.Is(err, ErrDraftRevisionConflict) {
		writeHeaderError(w, r, http.StatusConflict, CodeRevisionConflict, "Draft changed since it was read", false)
		return
	}
	if strings.Contains(err.Error(), "not found") {
		writeHeaderError(w, r, http.StatusNotFound, CodeNotFound, "Draft or message not found", false)
		return
	}
	var fault *Fault
	if errors.As(err, &fault) {
		status := http.StatusBadRequest
		if fault.Code == CodeInternal || fault.Code == CodeCredentialRepairRequired {
			status = http.StatusInternalServerError
		}
		writeHeaderError(w, r, status, fault.Code, fault.Message, fault.Retryable)
		return
	}
	writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Draft operation could not be completed", true)
}
