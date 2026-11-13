package app

import (
	"errors"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
)

func (a *App) remoteCalendarClient() (*automationclient.Client, error) {
	if a.mailboxService == nil {
		return nil, errors.New("remote mailbox service is not configured")
	}
	return a.mailboxService.getClient()
}

// Calendar_ListSources routes remote metadata to the helper without local stores.
func (a *App) Calendar_ListSources() ([]calendar.Source, error) {
	if !a.remoteServiceMode {
		if a.CalendarBridge == nil {
			return nil, errors.New("Calendar is unavailable")
		}
		return a.CalendarBridge.Calendar_ListSources()
	}
	c, err := a.remoteCalendarClient()
	if err != nil {
		return nil, err
	}
	return c.CalendarSources(a.accountContext())
}

// Calendar_ListCalendars reads one helper-owned Calendar source remotely.
func (a *App) Calendar_ListCalendars(sourceID string) ([]calendar.Calendar, error) {
	if !a.remoteServiceMode {
		if a.CalendarBridge == nil {
			return nil, errors.New("Calendar is unavailable")
		}
		return a.CalendarBridge.Calendar_ListCalendars(sourceID)
	}
	c, err := a.remoteCalendarClient()
	if err != nil {
		return nil, err
	}
	return c.CalendarCalendars(a.accountContext(), sourceID)
}

// Calendar_GetEvent sanitizes helper event bodies with the desktop render policy.
func (a *App) Calendar_GetEvent(id string) (*calendar.Event, error) {
	if !a.remoteServiceMode {
		if a.CalendarBridge == nil {
			return nil, errors.New("Calendar is unavailable")
		}
		return a.CalendarBridge.Calendar_GetEvent(id)
	}
	c, err := a.remoteCalendarClient()
	if err != nil {
		return nil, err
	}
	event, err := c.CalendarEvent(a.accountContext(), id)
	if err != nil || event == nil {
		return event, err
	}
	// Never trust a helper-supplied ready-to-render HTML field.
	event.DescriptionHTML = htmlCoreImpl{sanitizer: sharedSanitizer}.Sanitize(event.Description)
	return event, nil
}

// Calendar_ListEventsInRange keeps remote occurrence queries explicitly unavailable
// until recurrence work can be interrupted inside the evaluator's inner loops.
func (a *App) Calendar_ListEventsInRange(ids []string, from, to int64) ([]calendar.EventInstance, error) {
	if !a.remoteServiceMode {
		if a.CalendarBridge == nil {
			return nil, errors.New("Calendar is unavailable")
		}
		return a.CalendarBridge.Calendar_ListEventsInRange(ids, from, to)
	}
	return nil, errors.New("remote Calendar occurrence queries are unavailable pending context-aware recurrence support")
}
