package app

import (
	"errors"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

// calendarExtensionEnabled mirrors the local bridge's device-level gate.
func (a *App) calendarExtensionEnabled() bool {
	if a.settingsStore == nil {
		return false
	}
	enabled, err := a.settingsStore.IsExtensionEnabled("calendar")
	return err == nil && enabled
}

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
	if !a.calendarExtensionEnabled() {
		return nil, nil
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
	if !a.calendarExtensionEnabled() {
		return nil, nil
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
	if !a.calendarExtensionEnabled() {
		return nil, nil
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
	event.DescriptionHTML = ""
	if calendar.DescriptionIsHTML(event.Description) {
		event.DescriptionHTML = htmlCoreImpl{sanitizer: sharedSanitizer}.Sanitize(event.Description)
	}
	return event, nil
}

// Calendar_ListEventsInRange reads bounded occurrences without opening local stores.
func (a *App) Calendar_ListEventsInRange(ids []string, from, to int64) ([]calendar.EventInstance, error) {
	if !a.remoteServiceMode {
		if a.CalendarBridge == nil {
			return nil, errors.New("Calendar is unavailable")
		}
		return a.CalendarBridge.Calendar_ListEventsInRange(ids, from, to)
	}
	if !a.calendarExtensionEnabled() {
		return nil, nil
	}
	if len(ids) == 0 {
		return nil, nil
	}
	c, err := a.remoteCalendarClient()
	if err != nil {
		return nil, err
	}
	items, err := c.CalendarEvents(a.accountContext(), v1.CalendarRangeRequest{CalendarIDs: ids, FromUnix: from, ToUnix: to})
	if err != nil {
		return nil, err
	}
	// Like the local hot path, occurrence lists never supply ready-to-render bodies.
	for i := range items {
		items[i].DescriptionHTML = ""
	}
	return items, nil
}
