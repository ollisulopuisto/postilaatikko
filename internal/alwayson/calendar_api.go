package alwayson

import (
	"context"
	"errors"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/settings"
	"sync"
)

// calendarReadRuntime owns a lazy helper-side Calendar store. Read-only metadata
// does not start providers, drain pending writes or mark alarms as delivered.
// Provider sync and mutation lifecycle are a separate integration stage.
type calendarReadRuntime struct {
	mu       sync.Mutex
	store    *calendar.Store
	settings *settings.Store
	dataDir  string
	closed   bool
}

func newCalendarReadRuntime(db *database.DB, dataDir string) *calendarReadRuntime {
	return &calendarReadRuntime{settings: settings.NewStore(db), dataDir: dataDir}
}
func (r *calendarReadRuntime) withStore(ctx context.Context, read func(*calendar.Store) error) error {
	if ctx == nil {
		return errors.New("Calendar context unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.mu.TryLock() {
		return errors.New("Calendar read already in progress; retry shortly")
	}
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("Calendar runtime closed")
	}
	enabled, err := r.settings.IsExtensionEnabled("calendar")
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	if r.store == nil {
		r.store, err = calendar.NewStore(r.dataDir)
		if err != nil {
			return err
		}
	}
	return read(r.store)
}
func (r *calendarReadRuntime) CalendarSources(ctx context.Context) (out []calendar.Source, err error) {
	out = []calendar.Source{}
	err = r.withStore(ctx, func(s *calendar.Store) error { out, err = s.RemoteSources(ctx); return err })
	return
}
func (r *calendarReadRuntime) CalendarCalendars(ctx context.Context, id string) (out []calendar.Calendar, err error) {
	out = []calendar.Calendar{}
	err = r.withStore(ctx, func(s *calendar.Store) error { out, err = s.RemoteCalendars(ctx, id); return err })
	return
}
func (r *calendarReadRuntime) CalendarEvent(ctx context.Context, id string) (out *calendar.Event, err error) {
	err = r.withStore(ctx, func(s *calendar.Store) error { out, err = s.RemoteEvent(ctx, id); return err })
	return
}
func (r *calendarReadRuntime) CalendarEvents(ctx context.Context, request v1.CalendarRangeRequest) (out []calendar.EventInstance, err error) {
	if err = request.Validate(); err != nil {
		return nil, err
	}
	out = []calendar.EventInstance{}
	err = r.withStore(ctx, func(s *calendar.Store) error {
		out, err = s.RemoteEventsInRange(ctx, request.CalendarIDs, request.FromUnix, request.ToUnix)
		return err
	})
	return
}
func (r *calendarReadRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.store != nil {
		return r.store.Close()
	}
	return nil
}

var _ v1.CalendarReadOperations = (*calendarReadRuntime)(nil)
