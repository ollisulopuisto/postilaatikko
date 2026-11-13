package client

import (
	"context"
	"encoding/json"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"net/http"
	"net/url"
)

// CalendarSources reads helper-owned Calendar metadata under calendar.read.
func (c *Client) CalendarSources(ctx context.Context) ([]calendar.Source, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/calendar/sources", nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[[]calendar.Source]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// CalendarCalendars reads the calendars belonging to one helper source.
func (c *Client) CalendarCalendars(ctx context.Context, sourceID string) ([]calendar.Calendar, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/calendar/calendars?"+url.Values{"sourceId": {sourceID}}.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[[]calendar.Calendar]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// CalendarEvent reads one event. Renderers must sanitize any rich body locally.
func (c *Client) CalendarEvent(ctx context.Context, id string) (*calendar.Event, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/v1/calendar/events/"+url.PathEscape(id), nil, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[*calendar.Event]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// CalendarEvents reads a bounded occurrence window from helper-owned calendars.
func (c *Client) CalendarEvents(ctx context.Context, request v1.CalendarRangeRequest) ([]calendar.EventInstance, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	data, err := c.doRequest(ctx, http.MethodPost, "/v1/calendar/events/query", request, "")
	if err != nil {
		return nil, err
	}
	var response v1.Response[[]calendar.EventInstance]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}
