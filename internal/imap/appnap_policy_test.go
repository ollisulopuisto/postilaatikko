package imap

import (
	"os"
	"strings"
	"testing"
)

func TestIDLEDoesNotHoldProcessOutOfAppNap(t *testing.T) {
	source, err := os.ReadFile("idle.go")
	if err != nil {
		t.Fatalf("read IDLE implementation: %v", err)
	}
	if strings.Contains(string(source), "Watching for new mail (IMAP IDLE)") {
		t.Fatal("IMAP IDLE must not hold a process activity assertion; idle hidden apps should remain eligible for App Nap")
	}
}
