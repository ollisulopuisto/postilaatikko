package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountsAddRejectsSecretFlagsAndPipes(t *testing.T) {
	for _, args := range [][]string{{"--password", "never-log-this"}, {"--token", "never-log-this"}, {"--provider", "imap", "--email", "me@example.org", "--imap-host", "imap.example.org", "--smtp-host", "smtp.example.org"}} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w.Close()
		var output bytes.Buffer
		dir := t.TempDir()
		err = runAccountsAdd(append([]string{"--data", dir}, args...), dir, r, &output, &output)
		r.Close()
		if err == nil {
			t.Fatal("accepted secret flag or nonterminal")
		}
		if strings.Contains(output.String(), "never-log-this") || strings.Contains(err.Error(), "never-log-this") {
			t.Fatal("secret echoed")
		}
	}
}

func TestAccountsAddInvalidProviderDoesNotCreateProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")
	var output bytes.Buffer
	if err := runAccountsAdd([]string{"--provider", "invalid", "--data", dir}, dir, os.Stdin, &output, &output); err == nil {
		t.Fatal("accepted invalid provider")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("invalid options created profile")
	}
}
