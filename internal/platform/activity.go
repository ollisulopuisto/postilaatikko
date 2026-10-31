// Package platform provides platform-specific functionality
package platform

import "sync"

// ActivityOptions selects what an Activity prevents. The values mirror
// NSActivityOptions on macOS exactly; other platforms ignore them.
type ActivityOptions uint64

const (
	// ActivityBackground matches NSActivityBackground: the process stays
	// eligible for nothing but App Nap is prevented — no idle-display,
	// no idle-system and no termination prevention. Use for maintenance
	// work the user is not watching (a scheduled mail sync).
	ActivityBackground ActivityOptions = 0x000000FF
	// ActivityUserInitiated matches
	// NSActivityUserInitiatedAllowingIdleSystemSleep: App Nap and display
	// sleep are prevented, but the machine may still sleep on idle. Use
	// for short work the user is actively waiting for (an outgoing send).
	ActivityUserInitiated ActivityOptions = (1 << 40) | (1 << 14)
)

// Activity is one held power assertion. End releases it; End is idempotent
// and safe to call from any goroutine.
type Activity struct {
	once sync.Once
	end  func()
}

// End releases the assertion. The second and later calls do nothing.
func (a *Activity) End() {
	a.once.Do(a.end)
}

// activityBackend is the per-OS assertion primitive. begin returns a release
// function; it must never fail hard — a platform that cannot assert returns
// a no-op release, because mail must flow on every build.
type activityBackend interface {
	begin(options ActivityOptions, reason string) (release func())
}

// noopActivityBackend serves every platform without a power-assertion
// primitive (and tests that only exercise manager bookkeeping).
type noopActivityBackend struct{}

func (noopActivityBackend) begin(ActivityOptions, string) (release func()) {
	return func() {}
}

// ActivityManager hands out power assertions. It exists so that App Nap on
// macOS can be *supported* — the app stays nappable when it is truly idle —
// while the operations a hidden window must not interrupt (an in-flight
// sync, an outgoing send, the Undo Send countdown) explicitly hold the
// process awake. Without these assertions the app relies on macOS
// heuristics, and a napped app has its timers coalesced and its CPU
// throttled mid-operation.
type ActivityManager struct {
	backend activityBackend
}

// NewActivityManager returns a manager backed by the OS primitive
// (NSProcessInfo on macOS, a no-op elsewhere).
func NewActivityManager() *ActivityManager {
	return &ActivityManager{backend: newActivityBackend()}
}

// newActivityManagerForTest swaps the backend in tests.
func newActivityManagerForTest(backend activityBackend) *ActivityManager {
	return &ActivityManager{backend: backend}
}

// Begin holds an assertion with the given options until End is called on
// the returned Activity. reason shows up in Activity Monitor / pmset.
func (m *ActivityManager) Begin(options ActivityOptions, reason string) *Activity {
	if m == nil || m.backend == nil {
		return &Activity{end: func() {}}
	}
	return &Activity{end: m.backend.begin(options, reason)}
}

// BeginBackground holds an ActivityBackground assertion (see the constant).
func (m *ActivityManager) BeginBackground(reason string) *Activity {
	return m.Begin(ActivityBackground, reason)
}

// BeginUserInitiated holds an ActivityUserInitiated assertion (see the
// constant).
func (m *ActivityManager) BeginUserInitiated(reason string) *Activity {
	return m.Begin(ActivityUserInitiated, reason)
}
