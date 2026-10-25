package sync

import (
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/gmail"
)

func TestGmailStatusStartsUnknown(t *testing.T) {
	e := &Engine{gmailBlocked: map[string]time.Time{}, gmailStates: map[string]GmailAPIStatus{}}
	if got := e.GmailAPIStatus("acc"); got.State != GmailStateUnknown {
		t.Fatalf("state = %q, want unknown", got.State)
	}
}

// Every outcome of a Gmail call maps to one state, and recordGmailState reports
// a change only when the state actually changes, so the info log is written
// once per transition rather than on every search.
func TestRecordGmailStateReportsOnlyTransitions(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want GmailAPIState
	}{
		{"success", nil, GmailStateActive},
		{"success again", nil, GmailStateActive},
		{"api disabled", gmail.ErrAPIDisabled, GmailStateUnavailable},
		{"insufficient scope", gmail.ErrInsufficientScope, GmailStateUnavailable},
		{"reauth", gmail.ErrReauthRequired, GmailStateReauthRequired},
		{"not capable", gmail.ErrNotCapable, GmailStateNotUsed},
		{"transient keeps last known state", gmail.ErrTransient, GmailStateNotUsed},
	}
	e := &Engine{gmailBlocked: map[string]time.Time{}, gmailStates: map[string]GmailAPIStatus{}}
	wantChanged := []bool{true, false, true, false, true, true, false}
	for i, tt := range tests {
		changed := e.recordGmailState("acc", tt.err)
		if changed != wantChanged[i] {
			t.Errorf("%s: changed = %v, want %v", tt.name, changed, wantChanged[i])
		}
		if tt.want != "" && tt.err != gmail.ErrTransient && e.GmailAPIStatus("acc").State != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.name, e.GmailAPIStatus("acc").State, tt.want)
		}
	}
}

func TestGmailStatusIsPerAccount(t *testing.T) {
	e := &Engine{gmailBlocked: map[string]time.Time{}, gmailStates: map[string]GmailAPIStatus{}}
	e.recordGmailState("a", nil)
	e.recordGmailState("b", gmail.ErrAPIDisabled)
	if e.GmailAPIStatus("a").State != GmailStateActive || e.GmailAPIStatus("b").State != GmailStateUnavailable {
		t.Fatalf("a=%q b=%q", e.GmailAPIStatus("a").State, e.GmailAPIStatus("b").State)
	}
}
