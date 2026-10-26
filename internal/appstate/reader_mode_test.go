package appstate

import (
	"encoding/json"
	"strings"
	"testing"
)

// The reader mode is stored as given: the frontend owns the allowed values and
// treats anything it does not know as the default.
func TestUIStateKeepsReaderMode(t *testing.T) {
	data, err := json.Marshal(UIState{ReaderMode: "traditional"})
	if err != nil {
		t.Fatal(err)
	}
	var back UIState
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ReaderMode != "traditional" {
		t.Errorf("ReaderMode = %q after a round trip, want traditional", back.ReaderMode)
	}
}

func TestUIStateOmitsAnUnsetReaderMode(t *testing.T) {
	data, err := json.Marshal(UIState{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "readerMode") {
		t.Errorf("an unset reader mode must not be written, so older states stay unchanged: %s", data)
	}
	var old UIState
	if err := json.Unmarshal([]byte(`{"selectedFolderId":"inbox"}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.ReaderMode != "" {
		t.Errorf("a state saved before the reader mode existed reads as %q, want empty (the default)", old.ReaderMode)
	}
}
