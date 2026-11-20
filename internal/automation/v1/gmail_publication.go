package v1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/mailfilter"
)

// GmailPublicationRequest is an explicit account-owned native-filter operation.
// It never carries credentials, translates local substring rules or applies old mail.
type GmailPublicationRequest struct {
	Operation   string       `json:"operation"`
	AccountID   string       `json:"accountId"`
	RuleID      string       `json:"ruleId,omitempty"`
	ID          string       `json:"id,omitempty"`
	Fingerprint string       `json:"fingerprint,omitempty"`
	Filter      gmail.Filter `json:"filter,omitempty"`
}

// GmailPublicationReply contains non-secret context and durable attempt state.
type GmailPublicationReply struct {
	AccountID       string                       `json:"accountId"`
	Email           string                       `json:"email"`
	ConsentRequired bool                         `json:"consentRequired"`
	Labels          []gmail.Label                `json:"labels,omitempty"`
	Found           int                          `json:"found"`
	Truncated       bool                         `json:"truncated"`
	Fingerprint     string                       `json:"fingerprint,omitempty"`
	Publication     *mailfilter.GmailPublication `json:"publication,omitempty"`
}

// GmailPublicationOperations executes only in the account-owning helper.
type GmailPublicationOperations interface {
	GmailPublication(context.Context, GmailPublicationRequest) (GmailPublicationReply, error)
}

// MountGmailPublicationRoutes mounts bounded non-coalesced provider operations.
// Settings mutations require both mail.write and account.manage, not mail.read.
func MountGmailPublicationRoutes(mux RouteMux, ops GmailPublicationOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/filters/gmail", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodPost {
			writeFilterRuleError(w, r, 405, CodeInvalidRequest, "Method not allowed")
			return
		}
		var req GmailPublicationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "Malformed Gmail filter request")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "One request required")
			return
		}
		switch req.Operation {
		case "context", "preview", "status":
		case "prepare", "publish", "cancel", "reconcile":
			if !requireCap(w, r, CapabilityMailWrite) || !requireCap(w, r, CapabilityAccountManage) {
				return
			}
		default:
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, "Unknown Gmail filter operation")
			return
		}
		if ops == nil {
			writeFilterRuleError(w, r, 503, CodeInternal, "Gmail filter ownership unavailable")
			return
		}
		result, err := ops.GmailPublication(r.Context(), req)
		if err != nil {
			writeFilterRuleError(w, r, 400, CodeInvalidRequest, err.Error())
			return
		}
		writeFilterRuleResponse(w, r, result)
	})
}
