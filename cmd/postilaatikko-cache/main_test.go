package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackCommandRefusesMissingOrWrongDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.db")
	if err := run([]string{"--db", path, "--account", "a", "--message", "m"}); err == nil {
		t.Fatal("created missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("typo created a file")
	}
	if err := run(nil); err == nil {
		t.Fatal("missing ownership accepted")
	}
}
