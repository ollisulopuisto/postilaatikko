package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrCalendarOccurrenceUnavailable keeps the not-yet-safe evaluator off the API.
var ErrCalendarOccurrenceUnavailable = errors.New("remote occurrence queries require context-aware recurrence support")

// CalendarRangeRequest asks for bounded occurrences, not an unbounded export.
type CalendarRangeRequest struct {
	CalendarIDs []string `json:"calendarIds"`
	FromUnix    int64    `json:"fromUnix"`
	ToUnix      int64    `json:"toUnix"`
}

// Validate checks calendar identities and the time window against read limits.
func (r CalendarRangeRequest) Validate() error {
	if len(r.CalendarIDs) < 1 || len(r.CalendarIDs) > 32 || r.FromUnix < 0 || r.ToUnix <= r.FromUnix || r.ToUnix-r.FromUnix > 366*86400 {
		return errors.New("a positive range up to 366 days and 1..32 calendars are required")
	}
	for _, id := range r.CalendarIDs {
		if !calendarIDValid(id) {
			return errors.New("invalid calendar identity")
		}
	}
	return nil
}
func calendarIDValid(id string) bool {
	return id != "" && len(id) <= 200 && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "/\\\r\n\t")
}

// CalendarReadOperations is the helper's typed Calendar read boundary. Implementors
// must bound store work and recurrence expansion and preserve HTML sanitization.
type CalendarReadOperations interface {
	CalendarSources(context.Context) ([]calendar.Source, error)
	CalendarCalendars(context.Context, string) ([]calendar.Calendar, error)
	CalendarEvent(context.Context, string) (*calendar.Event, error)
	CalendarEvents(context.Context, CalendarRangeRequest) ([]calendar.EventInstance, error)
}

// MountCalendarReadRoutes requires an independent Calendar grant. It deliberately
// exposes no credential or arbitrary extension-method endpoint.
func MountCalendarReadRoutes(mux *http.ServeMux, ops CalendarReadOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	route := func(pattern string, operation func(context.Context, *http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !requireCap(w, r, CapabilityCalendarRead) {
				return
			}
			if ops == nil {
				writeHeaderError(w, r, http.StatusServiceUnavailable, CodeInternal, "Calendar service unavailable", true)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			data, err := operation(ctx, r)
			if err != nil {
				if errors.Is(err, ErrCalendarOccurrenceUnavailable) {
					writeHeaderError(w, r, http.StatusNotImplemented, CodeInvalidRequest, ErrCalendarOccurrenceUnavailable.Error(), false)
					return
				}
				var invalid *calendarRequestError
				if errors.As(err, &invalid) {
					writeHeaderError(w, r, 400, CodeInvalidRequest, invalid.Error(), false)
					return
				}
				writeHeaderError(w, r, 503, CodeInternal, "Calendar read could not complete within its limits", true)
				return
			}
			var buf calendarLimitedBuffer
			buf.limit = 8 << 20
			if err := json.NewEncoder(&buf).Encode(Response[any]{Data: data}); err != nil {
				writeHeaderError(w, r, 503, CodeInternal, "Calendar response exceeds read limits", false)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write(buf.Bytes())
		})
	}
	route("GET /v1/calendar/sources", func(ctx context.Context, r *http.Request) (any, error) { return ops.CalendarSources(ctx) })
	route("GET /v1/calendar/calendars", func(ctx context.Context, r *http.Request) (any, error) {
		id := r.URL.Query().Get("sourceId")
		if !calendarIDValid(id) {
			return nil, &calendarRequestError{"sourceId is required"}
		}
		return ops.CalendarCalendars(ctx, id)
	})
	route("GET /v1/calendar/events/{id}", func(ctx context.Context, r *http.Request) (any, error) {
		id := r.PathValue("id")
		if !calendarIDValid(id) {
			return nil, &calendarRequestError{"event identity is invalid"}
		}
		return ops.CalendarEvent(ctx, id)
	})
	route("POST /v1/calendar/events/query", func(ctx context.Context, r *http.Request) (any, error) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 8193))
		if err != nil || len(body) > 8192 {
			return nil, &calendarRequestError{"Calendar query is too large"}
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var request CalendarRangeRequest
		if err := decoder.Decode(&request); err != nil {
			return nil, &calendarRequestError{"invalid Calendar query"}
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, &calendarRequestError{"query must contain one JSON object"}
		}
		if err := request.Validate(); err != nil {
			return nil, &calendarRequestError{err.Error()}
		}
		return ops.CalendarEvents(ctx, request)
	})
}

type calendarRequestError struct{ message string }

func (e *calendarRequestError) Error() string { return e.message }

type calendarLimitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *calendarLimitedBuffer) Write(value []byte) (int, error) {
	if len(value) > b.limit-b.Len() {
		return 0, errors.New("Calendar response byte limit")
	}
	return b.Buffer.Write(value)
}
