package appstate

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestUIStatePinnedFolders checks that pinned folders round-trip through the
// persisted JSON and that state saved before pins existed still loads.
func TestUIStatePinnedFolders(t *testing.T) {
	pins := []PinnedFolder{
		{AccountID: "acc-1", FolderID: "f-inbox", Path: "INBOX", Name: "Inbox"},
		{AccountID: "acc-2", FolderID: "f-work", Path: "Work", Name: "Work"},
	}

	tests := []struct {
		name    string
		input   string
		want    []PinnedFolder
		wantKey bool
	}{
		{
			name:    "legacy state without the field loads with no pins",
			input:   `{"selectedAccountId":"acc-1"}`,
			want:    nil,
			wantKey: false,
		},
		{
			name:    "pins from another account decode in order",
			input:   `{"pinnedFolders":[{"accountId":"acc-1","folderId":"f-inbox","path":"INBOX","name":"Inbox"},{"accountId":"acc-2","folderId":"f-work","path":"Work","name":"Work"}]}`,
			want:    pins,
			wantKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var state UIState
			if err := json.Unmarshal([]byte(tt.input), &state); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(state.PinnedFolders, tt.want) {
				t.Fatalf("PinnedFolders = %#v, want %#v", state.PinnedFolders, tt.want)
			}
		})
	}
}

// TestUIStatePinnedFoldersOmittedWhenEmpty checks that an unpinned state does
// not write the field, so older builds reading the file see no change.
func TestUIStatePinnedFoldersOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(UIState{SelectedAccountID: "acc-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["pinnedFolders"]; ok {
		t.Fatalf("pinnedFolders written for empty state: %s", data)
	}
}
