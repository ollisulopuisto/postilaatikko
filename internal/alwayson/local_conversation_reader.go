package alwayson

import (
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/virtualfolder"
)

// LocalConversationReader runs the helper's conversation list and count reads
// in-process. The desktop uses it when no mailbox service is connected, so
// local mode executes exactly the code behind POST /v1/conversations and
// POST /v1/conversations/count instead of a parallel implementation.
type LocalConversationReader struct {
	ops *catalogReadOperations
}

// NewLocalConversationReader builds the reader over the given stores.
func NewLocalConversationReader(accounts *account.Store, folders *folder.Store, virtualFolders *virtualfolder.Store, messages *message.Store) *LocalConversationReader {
	return &LocalConversationReader{ops: newCatalogReadOperations(accounts, folders, virtualFolders, messages)}
}

// ListConversations returns one bounded conversation page.
func (r *LocalConversationReader) ListConversations(req v1.ConversationListRequest) (*v1.ConversationListPage, error) {
	page, err := v1.ExecuteConversationList(r.ops, req)
	if err != nil {
		return nil, err
	}
	return &page, nil
}

// CountConversations returns the exact conversation count for a scope.
func (r *LocalConversationReader) CountConversations(req v1.ConversationCountRequest) (int, error) {
	return v1.ExecuteConversationCount(r.ops, req)
}

// ConversationReadOperations is the helper's conversation read surface, as
// mounted behind /v1/conversations and /v1/conversations/count.
type ConversationReadOperations interface {
	v1.ConversationListOperations
	v1.ConversationListKeysetOperations
	v1.ConversationCountOperations
}

// NewConversationReadOperations returns the exact operations the helper
// mounts, so tests can compare the desktop's local mode against the real
// HTTP routes.
func NewConversationReadOperations(accounts *account.Store, folders *folder.Store, virtualFolders *virtualfolder.Store, messages *message.Store) ConversationReadOperations {
	return newCatalogReadOperations(accounts, folders, virtualFolders, messages)
}
