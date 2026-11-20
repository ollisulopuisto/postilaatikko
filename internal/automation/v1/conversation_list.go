package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

// ConversationListRequest asks for one bounded page of local conversation summaries.
type ConversationListRequest struct {
	Scope     Scope       `json:"scope"`
	SortOrder string      `json:"sortOrder,omitempty"`
	Filter    string      `json:"filter,omitempty"`
	Page      PageRequest `json:"page,omitempty"`
}

// ConversationListPage is a bounded page of conversation summaries.
type ConversationListPage struct {
	Items      []*message.Conversation `json:"items"`
	NextCursor string                  `json:"nextCursor,omitempty"`
	Complete   bool                    `json:"complete"`
}

// ConversationListOperations reads header-only conversation summaries.
type ConversationListOperations interface {
	GetFolder(folderID string) (*folder.Folder, error)
	ListConversationsByFolder(folderID string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error)
	ListConversationsUnifiedInbox(offset, limit int, sortOrder, filter string) ([]*message.Conversation, error)
	ListConversationsUnifiedFolder(folderType string, offset, limit int, sortOrder, filter string) ([]*message.Conversation, error)
}

// ConversationListKeysetOperations is implemented by operation sets that can
// serve keyset-paginated conversation pages (PERF-SC-005). The handler uses
// it for unfiltered newest-first lists, where each page costs O(page) instead
// of O(folder); offset pagination remains for every other combination.
type ConversationListKeysetOperations interface {
	ListConversationsByFolderKeyset(folderID, cursor string, limit int) ([]*message.Conversation, string, error)
	ListConversationsUnifiedInboxKeyset(cursor string, limit int) ([]*message.Conversation, string, error)
	ListConversationsUnifiedFolderKeyset(folderType, cursor string, limit int) ([]*message.Conversation, string, error)
}

// MountConversationListRoute mounts the authenticated conversation-list route.
func MountConversationListRoute(mux RouteMux, ops ConversationListOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		if r.Method != http.MethodPost {
			writeHeaderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed", false)
			return
		}
		handleConversationList(w, r, ops)
	})
}

func handleConversationList(w http.ResponseWriter, r *http.Request, ops ConversationListOperations) {
	var req ConversationListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeHeaderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed request body", false)
		return
	}
	page, reqErr := executeConversationList(ops, req)
	if reqErr != nil {
		writeHeaderError(w, r, reqErr.status, reqErr.code, reqErr.message, reqErr.retryable)
		return
	}
	writeAPIResponse(w, r, http.StatusOK, page)
}

// requestError carries one transport-shaped failure so shared read logic can
// be used by both individual routes and aggregate handlers.
type requestError struct {
	status    int
	code      ErrorCode
	message   string
	retryable bool
}

// executeConversationList runs the validated, bounded conversation-page read
// behind POST /v1/conversations. The workspace bootstrap snapshot reuses it
// so both paths apply identical scope validation, defaults, bounds, and
// body-stripping rules.
func executeConversationList(ops ConversationListOperations, req ConversationListRequest) (ConversationListPage, *requestError) {
	if err := req.Scope.Validate(); err != nil {
		return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Invalid conversation scope", false}
	}
	if req.Scope.Kind != views.ScopeFolder && req.Scope.Kind != views.ScopeAllInboxes && req.Scope.Kind != views.ScopeUnifiedFolder {
		return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Conversation lists support folder, unified_folder, and all_inboxes scopes", false}
	}
	if req.SortOrder == "" {
		req.SortOrder = "newest"
	}
	if req.SortOrder != "newest" && req.SortOrder != "oldest" {
		return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, "sortOrder must be newest or oldest", false}
	}
	if !validConversationListFilter(req.Filter) {
		return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, "Unsupported conversation filter", false}
	}
	pageRequest, err := req.Page.Normalized()
	if err != nil {
		return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, err.Error(), false}
	}
	// Keyset path (PERF-SC-005): unfiltered newest-first lists paginate by
	// opaque keyset cursor. Legacy numeric cursors keep offset semantics, so
	// existing clients are unaffected; a keyset cursor never enters the
	// offset path.
	keysetCursor := pageRequest.Cursor
	useKeyset := req.Filter == "" && req.SortOrder == "newest" &&
		(keysetCursor == "" || strings.HasPrefix(keysetCursor, message.KeysetCursorPrefix))
	if useKeyset {
		if kop, ok := ops.(ConversationListKeysetOperations); ok {
			page, keysetHandled, reqErr := executeConversationListKeyset(kop, ops, req, keysetCursor, pageRequest.Limit)
			if reqErr != nil {
				return ConversationListPage{}, reqErr
			}
			if keysetHandled {
				return page, nil
			}
		}
	}

	offset := 0
	if pageRequest.Cursor != "" {
		offset, err = strconv.Atoi(pageRequest.Cursor)
		if err != nil || offset < 0 || offset > MaximumMessageOffset {
			return ConversationListPage{}, &requestError{http.StatusBadRequest, CodeInvalidRequest, "cursor is invalid or exceeds the maximum offset", false}
		}
	}

	var items []*message.Conversation
	var listErr error
	if req.Scope.Kind == views.ScopeFolder {
		f, err := ops.GetFolder(req.Scope.FolderID)
		if err != nil {
			return ConversationListPage{}, &requestError{http.StatusInternalServerError, CodeInternal, "Failed to load folder", true}
		}
		if f == nil || f.AccountID != req.Scope.AccountID {
			return ConversationListPage{}, &requestError{http.StatusNotFound, CodeNotFound, "Folder not found for account", false}
		}
		items, listErr = ops.ListConversationsByFolder(req.Scope.FolderID, offset, pageRequest.Limit+1, req.SortOrder, req.Filter)
	} else if req.Scope.Kind == views.ScopeAllInboxes {
		items, listErr = ops.ListConversationsUnifiedInbox(offset, pageRequest.Limit+1, req.SortOrder, req.Filter)
	} else {
		items, listErr = ops.ListConversationsUnifiedFolder(req.Scope.FolderType, offset, pageRequest.Limit+1, req.SortOrder, req.Filter)
	}
	if listErr != nil {
		return ConversationListPage{}, &requestError{http.StatusInternalServerError, CodeInternal, "Failed to load conversations", true}
	}

	page := ConversationListPage{Items: items, Complete: true}
	if len(items) > pageRequest.Limit {
		page.Items = items[:pageRequest.Limit]
		page.Complete = false
		page.NextCursor = strconv.Itoa(offset + pageRequest.Limit)
	}
	for _, item := range page.Items {
		if item == nil {
			continue
		}
		if req.Scope.Kind == views.ScopeFolder {
			item.AccountID = req.Scope.AccountID
			item.FolderID = req.Scope.FolderID
		}
		// The list contract never returns fetched message bodies.
		item.Messages = nil
	}
	if page.Items == nil {
		page.Items = []*message.Conversation{}
	}
	return page, nil
}

// executeConversationListKeyset runs the keyset-paginated conversation read.
// keysetHandled reports whether the scope is served by the keyset path;
// unified archive and read-later scopes merge several sources per page and
// stay on the offset path for now.
func executeConversationListKeyset(kop ConversationListKeysetOperations, ops ConversationListOperations, req ConversationListRequest, cursor string, limit int) (ConversationListPage, bool, *requestError) {
	var items []*message.Conversation
	var next string
	var listErr error
	switch req.Scope.Kind {
	case views.ScopeFolder:
		f, err := ops.GetFolder(req.Scope.FolderID)
		if err != nil {
			return ConversationListPage{}, false, &requestError{http.StatusInternalServerError, CodeInternal, "Failed to load folder", true}
		}
		if f == nil || f.AccountID != req.Scope.AccountID {
			return ConversationListPage{}, false, &requestError{http.StatusNotFound, CodeNotFound, "Folder not found for account", false}
		}
		items, next, listErr = kop.ListConversationsByFolderKeyset(req.Scope.FolderID, cursor, limit)
	case views.ScopeAllInboxes:
		items, next, listErr = kop.ListConversationsUnifiedInboxKeyset(cursor, limit)
	case views.ScopeUnifiedFolder:
		if !keysetUnifiedFolderType(req.Scope.FolderType) {
			return ConversationListPage{}, false, nil
		}
		items, next, listErr = kop.ListConversationsUnifiedFolderKeyset(req.Scope.FolderType, cursor, limit)
	default:
		return ConversationListPage{}, false, nil
	}
	if listErr != nil {
		status := http.StatusInternalServerError
		code := ErrorCode(CodeInternal)
		if strings.Contains(listErr.Error(), "keyset cursor") {
			status = http.StatusBadRequest
			code = CodeInvalidRequest
		}
		return ConversationListPage{}, false, &requestError{status, code, "Failed to load conversations", status == http.StatusInternalServerError}
	}
	page := ConversationListPage{Items: items, Complete: next == "", NextCursor: next}
	for _, item := range page.Items {
		if item == nil {
			continue
		}
		if req.Scope.Kind == views.ScopeFolder {
			item.AccountID = req.Scope.AccountID
			item.FolderID = req.Scope.FolderID
		}
		// The list contract never returns fetched message bodies.
		item.Messages = nil
	}
	if page.Items == nil {
		page.Items = []*message.Conversation{}
	}
	return page, true, nil
}

// keysetUnifiedFolderType reports which unified folder types the keyset path
// serves: the ones backed by one physical folder list. Archive merges a
// physical and a virtual listing per page, and the read-later queues have
// their own indexed store paths; all three stay on offset pagination.
func keysetUnifiedFolderType(folderType string) bool {
	switch folderType {
	case "drafts", "sent", "trash", "starred", "spam":
		return true
	default:
		return false
	}
}

func validConversationListFilter(filter string) bool {
	if filter == "" {
		return true
	}
	allowed := map[string]bool{
		"unread":      true,
		"needsMe":     true,
		"starred":     true,
		"attachments": true,
		"readLater":   true,
		"replyLater":  true,
	}
	seen := make(map[string]bool)
	for _, part := range strings.Split(filter, "+") {
		if !allowed[part] || seen[part] {
			return false
		}
		seen[part] = true
	}
	return true
}
