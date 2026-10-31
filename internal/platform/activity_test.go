package platform

import (
	"sync"
	"testing"
)

// fakeActivityBackend records begin/end calls so the manager's bookkeeping
// can be tested on any OS.
type fakeActivityBackend struct {
	mu      sync.Mutex
	begins  []fakeBegin
	ends    int
	release chan struct{}
}

type fakeBegin struct {
	options ActivityOptions
	reason  string
}

func (f *fakeActivityBackend) begin(options ActivityOptions, reason string) (release func()) {
	f.mu.Lock()
	f.begins = append(f.begins, fakeBegin{options: options, reason: reason})
	f.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			f.ends++
			if f.release != nil {
				close(f.release)
			}
			f.mu.Unlock()
		})
	}
}

func (f *fakeActivityBackend) snapshot() ([]fakeBegin, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeBegin(nil), f.begins...), f.ends
}

func TestActivityManagerBeginEndPairing(t *testing.T) {
	backend := &fakeActivityBackend{}
	m := newActivityManagerForTest(backend)

	a := m.Begin(ActivityBackground, "test work")
	begins, ends := backend.snapshot()
	if len(begins) != 1 || begins[0].options != ActivityBackground || begins[0].reason != "test work" {
		t.Fatalf("begin not recorded as expected: %+v", begins)
	}
	if ends != 0 {
		t.Fatalf("ended before End: %d", ends)
	}

	a.End()
	_, ends = backend.snapshot()
	if ends != 1 {
		t.Fatalf("End did not release: %d", ends)
	}

	// End is idempotent: a second call must not double-release.
	a.End()
	_, ends = backend.snapshot()
	if ends != 1 {
		t.Fatalf("second End released again: %d", ends)
	}
}

func TestActivityManagerOptionHelpers(t *testing.T) {
	backend := &fakeActivityBackend{}
	m := newActivityManagerForTest(backend)

	m.BeginBackground("sync")
	m.BeginUserInitiated("send")

	begins, _ := backend.snapshot()
	if len(begins) != 2 {
		t.Fatalf("expected 2 begins, got %d", len(begins))
	}
	if begins[0].options != ActivityBackground {
		t.Fatalf("BeginBackground used options %#x", begins[0].options)
	}
	if begins[1].options != ActivityUserInitiated {
		t.Fatalf("BeginUserInitiated used options %#x", begins[1].options)
	}
}

func TestActivityManagerNilSafe(t *testing.T) {
	var m *ActivityManager
	// A nil manager (e.g. a partially constructed App in a test) must still
	// return an Activity whose End is safe.
	a := m.BeginBackground("anything")
	a.End()
	a.End()
}

// The macOS option values must match NSActivityOptions exactly; a typo here
// silently changes what the OS prevents. Pinned from NSProcessInfo.h:
// NSActivityIdleDisplaySleepDisabled = 1<<40,
// NSActivityIdleSystemSleepDisabled = 1<<20,
// NSActivitySuddenTerminationDisabled = 1<<14,
// NSActivityUserInitiated = 0x00FFFFFF | NSActivityIdleSystemSleepDisabled,
// NSActivityUserInitiatedAllowingIdleSystemSleep = 0x00FFFFFF,
// NSActivityBackground = 0xFF.
func TestActivityOptionValuesMatchNSActivityOptions(t *testing.T) {
	if ActivityBackground != 0xFF {
		t.Fatalf("ActivityBackground = %#x, want 0xFF", ActivityBackground)
	}
	const wantUserInitiated = ActivityOptions(0x00FFFFFF)
	if ActivityUserInitiated != wantUserInitiated {
		t.Fatalf("ActivityUserInitiated = %#x, want %#x", ActivityUserInitiated, wantUserInitiated)
	}
	// Note: 0x00FFFFFF is a packed field, not a composition of the
	// individual flags above - bit 20 being set inside it does NOT assert
	// idle-system-sleep prevention. Apple's own UserInitiated definition
	// ORs IdleSystemSleepDisabled on top of the same 0x00FFFFFF field,
	// which would be redundant if the bit were live there. The
	// AllowingIdleSystemSleep semantics come from the API contract.

}
