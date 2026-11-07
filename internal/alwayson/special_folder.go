package alwayson

import (
	"fmt"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
)

func resolveSpecialFolder(accounts *account.Store, folders *folder.Store, accountID string, folderType folder.Type) (*folder.Folder, error) {
	acc, err := accounts.Get(accountID)
	if err != nil {
		return nil, fmt.Errorf("load account %s: %w", accountID, err)
	}
	if acc == nil {
		return nil, fmt.Errorf("account not found: %s", accountID)
	}
	if !acc.Enabled {
		return nil, nil
	}

	if mappedPath := acc.GetFolderMapping(string(folderType)); mappedPath != "" {
		mapped, err := folders.GetByPath(accountID, mappedPath)
		if err != nil {
			return nil, err
		}
		if mapped != nil {
			return mapped, nil
		}
	}
	return folders.GetByType(accountID, folderType)
}

func resolveSpecialFolderIDs(accounts *account.Store, folders *folder.Store, folderType folder.Type) ([]string, error) {
	accountList, err := accounts.List()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(accountList))
	for _, acc := range accountList {
		if acc == nil || !acc.Enabled {
			continue
		}
		item, err := resolveSpecialFolder(accounts, folders, acc.ID, folderType)
		if err != nil {
			return nil, err
		}
		if item != nil {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}
