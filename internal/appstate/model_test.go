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

// TestUIStateSidebarLayout checks that the sidebar block order and hidden set
// round-trip, and that an unset layout is not written, so older builds see no change.
func TestUIStateSidebarLayout(t *testing.T) {
	tests := []struct {
		name       string
		state      UIState
		wantOrder  []string
		wantHidden []string
	}{
		{
			name:       "legacy state without layout fields loads with none",
			state:      UIState{},
			wantOrder:  nil,
			wantHidden: nil,
		},
		{
			name:       "custom order and hidden blocks round-trip",
			state:      UIState{SidebarOrder: []string{"calendar", "unified"}, SidebarHidden: []string{"virtual"}},
			wantOrder:  []string{"calendar", "unified"},
			wantHidden: []string{"virtual"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got UIState
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(got.SidebarOrder, tt.wantOrder) {
				t.Fatalf("SidebarOrder = %#v, want %#v (json %s)", got.SidebarOrder, tt.wantOrder, data)
			}
			if !reflect.DeepEqual(got.SidebarHidden, tt.wantHidden) {
				t.Fatalf("SidebarHidden = %#v, want %#v (json %s)", got.SidebarHidden, tt.wantHidden, data)
			}
		})
	}
}
