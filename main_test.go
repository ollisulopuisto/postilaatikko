package main

import (
	"os"
	"testing"
)

func TestDebugModeDefault(t *testing.T) {
	// Ensure the env var is not set so we only test the default flag value
	t.Setenv("AERION_DEBUG", "")

	if DebugMode() {
		t.Error("DebugMode() = true, want false by default")
	}
}

func TestDebugModeEnvVar(t *testing.T) {
	t.Setenv("AERION_DEBUG", "1")

	if !DebugMode() {
		t.Error("DebugMode() = false, want true when AERION_DEBUG=1")
	}

	// Verify env var was actually set (sanity check)
	if os.Getenv("AERION_DEBUG") != "1" {
		t.Error("AERION_DEBUG env var not set correctly")
	}
}

func TestPerfMailtoCombinationRejectedBeforeActivation(t *testing.T) {
	for _, tt := range []struct {
		perf    bool
		mailto  string
		wantErr bool
	}{{true, "mailto:test@example.invalid", true}, {true, "", false}, {false, "mailto:test@example.invalid", false}} {
		if err := validatePerfArguments(tt.perf, tt.mailto); (err != nil) != tt.wantErr {
			t.Fatalf("combination %+v: %v", tt, err)
		}
	}
}
