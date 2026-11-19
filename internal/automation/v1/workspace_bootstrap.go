package v1

import (
	"encoding/json"
	"net/http"
)

// Workspace bootstrap (PERF-SC-003). The remote desktop's first usable paint
// otherwise costs six sequential round trips: capabilities, accounts,
// folders, views, unread counts, and the first conversation page. This route
// returns the five mailbox sections in one request so a high-RTT link pays
// the latency once.
//
// Snapshot semantics: the sections are separate sequential reads against the
// service layer, exactly as the individual routes perform them today. There
// is no cross-section revision or isolation; clients that need coherence
// re-read the section that matters. The response carries no revision field
// for that reason. Partial failures are explicit: a failing section leaves
// its field empty and adds an entry to Errors, while successful sections are
// still returned.
//
// Bounds: the initial page goes through the same validation, defaults, and
// page caps as POST /v1/conversations (executeConversationList); folders are
// queried per account; the response never contains bodies, attachment bytes,
// credentials, or local paths (the same safe summaries the individual
// catalog routes emit).

// WorkspaceBootstrapRequest identifies the initial scope and page limit.
type WorkspaceBootstrapRequest struct {
	// FoldersAccountID limits the folder catalog to one account; empty
	// queries every account, like GET /v1/folders without accountId.
	FoldersAccountID string `json:"foldersAccountId,omitempty"`
	// InitialPage describes the first conversation page: scope, sort,
	// filter, and bounded page size.
	InitialPage *ConversationListRequest `json:"initialPage,omitempty"`
}

// WorkspaceBootstrapResponse carries the mailbox sections needed for first
// usable paint. Section fields mirror the standalone routes' wire shapes.
type WorkspaceBootstrapResponse struct {
	Accounts     []AccountSummary      `json:"accounts"`
	Folders      []FolderSummary       `json:"folders"`
	Views        []ViewSummary         `json:"views"`
	UnreadCounts map[string]int        `json:"unreadCounts"`
	InitialPage  *ConversationListPage `json:"initialPage,omitempty"`
	// Errors maps a failed section ("accounts", "folders", "views",
	// "unreadCounts", "initialPage") to a client-safe message. A missing
	// section capability rejects the whole request before any read.
	Errors map[string]string `json:"errors,omitempty"`
}

// WorkspaceBootstrapOperations composes the service-layer reads behind the
// individual catalog routes; the bootstrap reuses them rather than
// duplicating their business rules.
type WorkspaceBootstrapOperations interface {
	CatalogOperations
	ConversationListOperations
	VirtualFolderUnreadCountOperations
}

// MountWorkspaceBootstrapRoute mounts the authenticated one-request startup
// snapshot.
func MountWorkspaceBootstrapRoute(mux RouteMux, ops WorkspaceBootstrapOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("POST /v1/workspace/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		var req WorkspaceBootstrapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed request body", false)
			return
		}

		resp := WorkspaceBootstrapResponse{
			Accounts:     []AccountSummary{},
			Folders:      []FolderSummary{},
			Views:        []ViewSummary{},
			UnreadCounts: map[string]int{},
		}
		sectionErr := func(section string, err *requestError) {
			if resp.Errors == nil {
				resp.Errors = map[string]string{}
			}
			resp.Errors[section] = err.message
		}

		accounts, err := ops.GetAccounts()
		if err != nil {
			sectionErr("accounts", &requestError{message: "Failed to load accounts"})
		} else {
			for _, acc := range accounts {
				resp.Accounts = append(resp.Accounts, accountSummaryFrom(acc))
			}
		}

		accountIDs := []string{}
		if req.FoldersAccountID != "" {
			accountIDs = append(accountIDs, req.FoldersAccountID)
		} else if resp.Errors != nil {
			// The account list failed; without it there is no bounded way to
			// enumerate folder queries, so the section is skipped explicitly.
			sectionErr("folders", &requestError{message: "Skipped: accounts unavailable"})
		} else {
			for _, acc := range resp.Accounts {
				accountIDs = append(accountIDs, acc.ID)
			}
		}
		foldersFailed := false
		for _, accountID := range accountIDs {
			folders, ferr := ops.GetFolders(accountID)
			if ferr != nil {
				// Matches GET /v1/folders: one failing account does not
				// hide the others, but the bootstrap reports it explicitly.
				foldersFailed = true
				continue
			}
			for _, f := range folders {
				resp.Folders = append(resp.Folders, folderSummaryFrom(f))
			}
		}
		if foldersFailed {
			sectionErr("folders", &requestError{message: "Failed to load folders for at least one account"})
		}

		virtualFolders, err := ops.GetVirtualFolders()
		if err != nil {
			sectionErr("views", &requestError{message: "Failed to load virtual views"})
		} else {
			for _, vf := range virtualFolders {
				resp.Views = append(resp.Views, viewSummaryFrom(vf))
			}
		}

		counts, err := ops.GetVirtualFolderUnreadCounts()
		if err != nil {
			sectionErr("unreadCounts", &requestError{message: "Could not load virtual-folder unread counts"})
		} else if counts != nil {
			resp.UnreadCounts = counts
		}

		if req.InitialPage != nil {
			page, reqErr := executeConversationList(ops, *req.InitialPage)
			if reqErr != nil {
				sectionErr("initialPage", reqErr)
			} else {
				resp.InitialPage = &page
			}
		}

		writeAPIResponse(w, r, http.StatusOK, resp)
	})
}
