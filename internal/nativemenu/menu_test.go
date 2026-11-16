package nativemenu

import (
	"reflect"
	"testing"
)

func TestCatalogAndDisabledStartup(t *testing.T) {
	want := []string{"File", "Edit", "View", "Message", "Mailbox", "Format", "Go", "Window", "Help"}
	var got []string
	c := New()
	for _, item := range Catalog() {
		got = append(got, item.Label)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("menu order: %v", got)
	}
	for id, items := range c.items {
		if id == "" {
			t.Fatal("empty action id")
		}
		for _, item := range items {
			if !item.Disabled {
				t.Fatalf("startup action %s enabled before frontend readiness", id)
			}
		}
	}
	if len(c.items["calendar.new"]) != 2 {
		t.Fatal("shared New Event entry missing")
	}
	// Preserve only the four native edit accelerators. Other existing bindings
	// stay with their current context-aware webview handlers.
	for id, items := range c.items {
		for _, item := range items {
			edit := id == "edit.cut" || id == "edit.copy" || id == "edit.paste" || id == "edit.selectAll"
			if (item.Accelerator != nil) != edit {
				t.Fatalf("unexpected accelerator for %s", id)
			}
		}
	}
}
func TestStateReconciliationAcrossSelectionAndPageReload(t *testing.T) {
	c := New()
	if !c.applyState(State{Epoch: 100, Enabled: []string{"calendar.new", "composer.send", "not-a-catalog-id"}}) {
		t.Fatal("first enable did not refresh")
	}
	for _, item := range c.items["calendar.new"] {
		if item.Disabled {
			t.Fatal("duplicate entry did not enable")
		}
	}
	if c.applyState(State{Epoch: 100, Enabled: []string{"calendar.new", "composer.send"}}) {
		t.Fatal("identical state needlessly refreshed")
	}
	if c.applyState(State{Epoch: 99, Enabled: []string{"mail.trash"}}) {
		t.Fatal("stale state refreshed menu")
	}
	if !c.items["mail.trash"][0].Disabled {
		t.Fatal("stale state enabled destructive action")
	}
	if !c.applyState(State{Epoch: 200, Enabled: []string{"mail.reply"}}) {
		t.Fatal("new page state did not refresh")
	}
	if !c.items["composer.send"][0].Disabled {
		t.Fatal("old composer Send remained enabled")
	}
	for _, item := range c.items["calendar.new"] {
		if !item.Disabled {
			t.Fatal("old duplicate entry remained enabled")
		}
	}
	if c.applyState(State{Epoch: 101, Enabled: []string{"composer.send"}}) {
		t.Fatal("delayed old page update accepted")
	}
	if !c.items["composer.send"][0].Disabled {
		t.Fatal("delayed old page enabled Send")
	}
}
