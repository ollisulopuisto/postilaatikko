//go:build linux

package main

import (
	"errors"
	"github.com/zalando/go-keyring"
	"os"
	"testing"
)

func TestExplicitFileIdentityDoesNotNeedOrFallbackToKeyring(t *testing.T) {
	keyring.MockInitWithError(errors.New("no Secret Service"))
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadHelperIdentity("encrypted-file", dir, "studio"); err == nil {
		t.Fatal("missing file silently generated")
	}
	created, err := createHelperIdentity("encrypted-file", dir, "studio")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := loadHelperIdentity("encrypted-file", dir, "studio")
	if err != nil || created.PublicKey() != restored.PublicKey() {
		t.Fatal("file backend failed")
	}
	if _, err := loadHelperIdentity("keyring", dir, "studio"); err == nil {
		t.Fatal("keyring silently loaded file backend")
	}
}
