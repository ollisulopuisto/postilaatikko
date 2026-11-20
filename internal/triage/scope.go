package triage

import (
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
)

// DefaultFolderScope resolves the real folders used by the cross-account
// triage view. Excluded system folders honor explicit account mappings. All
// Mail is omitted because including it would reintroduce archived copies.
func DefaultFolderScope(accounts *account.Store, folders *folder.Store) ([]*folder.Folder, error) {
	list, err := accounts.List()
	if err != nil {
		return nil, err
	}
	result := []*folder.Folder{}
	for _, acc := range list {
		if acc == nil || !acc.Enabled {
			continue
		}
		items, err := folders.List(acc.ID)
		if err != nil {
			return nil, err
		}
		excluded := map[string]bool{}
		for _, kind := range []folder.Type{folder.TypeSpam, folder.TypeSent, folder.TypeDrafts, folder.TypeArchive, folder.TypeTrash, folder.TypeAll} {
			path := acc.GetFolderMapping(string(kind))
			for _, item := range items {
				if item.Type == kind || (path != "" && item.Path == path) {
					excluded[item.ID] = true
				}
			}
		}
		// Hidden ancestors also hide their descendants from aggregate triage.
		for _, item := range items {
			if item.Hidden {
				excluded[item.ID] = true
			}
		}
		for changed := true; changed; {
			changed = false
			for _, item := range items {
				if !excluded[item.ID] && excluded[item.ParentID] {
					excluded[item.ID] = true
					changed = true
				}
			}
		}
		for _, item := range items {
			if !excluded[item.ID] {
				result = append(result, item)
			}
		}
	}
	return result, nil
}
