package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyAutostartCleanupPreservesUnrelatedEntries(t *testing.T) {
	dir := t.TempDir()
	for _, id := range append(append([]string{}, legacyDesktopIDs...), "other.app") {
		if err := os.WriteFile(filepath.Join(dir, id+".desktop"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !hasLegacyAutostart(dir, ".desktop") {
		t.Fatal("missed existing legacy startup")
	}
	if err := removeLegacyAutostart(dir, ".desktop"); err != nil {
		t.Fatal(err)
	}
	if hasLegacyAutostart(dir, ".desktop") {
		t.Fatal("old login entry remains")
	}
	if _, err := os.Stat(filepath.Join(dir, "other.app.desktop")); err != nil {
		t.Fatal("unrelated entry changed")
	}
	if err := removeLegacyAutostart(dir, ".desktop"); err != nil {
		t.Fatal("cleanup not idempotent")
	}
}
