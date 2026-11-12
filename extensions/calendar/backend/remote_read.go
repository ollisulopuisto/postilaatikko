package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RemoteEventsInRange is the bounded, store-only query for a trusted helper.
// It does not start a syncer, construct provider clients or invoke desktop APIs.
// Oversized or dense recurrence data fails explicitly rather than appearing as
// a complete but silently truncated calendar.
func (s *Store) RemoteEventsInRange(ctx context.Context, calendarIDs []string, fromUnix, toUnix int64) ([]EventInstance, error) {
	if ctx == nil || len(calendarIDs) < 1 || len(calendarIDs) > 32 || fromUnix < 0 || toUnix <= fromUnix || toUnix-fromUnix > 366*86400 {
		return nil, errors.New("calendar range requires 1..32 calendars and a positive window no larger than 366 days")
	}
	for _, id := range calendarIDs {
		if id == "" || len(id) > 200 {
			return nil, errors.New("invalid calendar identity")
		}
	}
	if s == nil {
		return nil, errors.New("calendar store unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	placeholders := make([]string, len(calendarIDs))
	args := make([]any, len(calendarIDs))
	for i, id := range calendarIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	// Select only IDs and bounded length metadata before loading any ICS bodies.
	// A transaction holds a consistent snapshot for subsequent per-event reads.
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	q := fmt.Sprintf(`SELECT e.id, length(CAST(COALESCE(e.ics_blob,'') AS BLOB))+length(CAST(COALESCE(e.summary,'') AS BLOB))+length(CAST(COALESCE(e.description,'') AS BLOB))+length(CAST(COALESCE(e.attendees_json,'') AS BLOB))+length(CAST(COALESCE(e.organizer_json,'') AS BLOB))+length(CAST(COALESCE(e.location,'') AS BLOB))+length(CAST(COALESCE(e.href,'') AS BLOB))+length(CAST(COALESCE(e.etag,'') AS BLOB))+length(CAST(COALESCE(e.provider_event_id,'') AS BLOB))+length(CAST(COALESCE(e.uid,'') AS BLOB))+length(CAST(COALESCE(e.rrule_text,'') AS BLOB))+length(CAST(COALESCE(e.tz_name,'') AS BLOB))+length(CAST(COALESCE(e.transparency,'') AS BLOB))+length(CAST(COALESCE(e.visibility,'') AS BLOB)) FROM events e JOIN calendars c ON c.id=e.calendar_id WHERE e.calendar_id IN (%s) AND c.visible=1 ORDER BY e.id LIMIT 1001`, strings.Join(placeholders, ","))
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	sizes := map[string]int64{}
	total := int64(0)
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			rows.Close()
			return nil, err
		}
		total += size
		if len(ids) >= 1000 || size > 1<<20 || total > 8<<20 {
			rows.Close()
			return nil, errors.New("calendar data exceeds remote read budget; narrow the selected calendars")
		}
		ids = append(ids, id)
		sizes[id] = size
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// The existing Store helpers use their own connections. Copy bounded snapshot
	// rows into memory while this transaction is open, then expand without SQL.
	out := []EventInstance{}
	responseBudget := int64(0)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ev, err := getEventForRemoteSnapshot(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		overrides, err := getOverridesForRemoteSnapshot(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		instances, err := ExpandInRangeBounded(ctx, *ev, overrides, time.Unix(fromUnix, 0), time.Unix(toUnix, 0), 1000-len(out), 100000)
		if err != nil {
			return nil, err
		}
		overrideBytes := int64(0)
		for _, override := range overrides {
			overrideBytes += int64(len(override.ICSBlob))
		}
		responseBudget += (6*(sizes[id]+overrideBytes) + 4096) * int64(len(instances))
		if responseBudget > 8<<20 {
			return nil, errors.New("calendar response byte budget exceeded")
		}
		if len(out)+len(instances) > 1000 {
			return nil, errors.New("calendar instance budget exceeded")
		}
		out = append(out, instances...)
	}
	return out, nil
}

func getEventForRemoteSnapshot(ctx context.Context, tx *sql.Tx, id string) (*Event, error) {
	row := tx.QueryRowContext(ctx, `SELECT id, calendar_id, uid, etag, href, provider_event_id,
 summary, COALESCE(description,''), COALESCE(location,''),dtstart_unix,dtend_unix,is_all_day,COALESCE(tz_name,''),
 COALESCE(rrule_text,''),COALESCE(transparency,'busy'),COALESCE(visibility,'public'),ics_blob,attendees_json,organizer_json FROM events WHERE id=?`, id)
	ev := &Event{}
	var allDay int
	var attendees string
	var organizer sql.NullString
	if err := row.Scan(&ev.ID, &ev.CalendarID, &ev.UID, &ev.ETag, &ev.Href, &ev.ProviderEventID, &ev.Summary, &ev.Description, &ev.Location, &ev.DTStartUnix, &ev.DTEndUnix, &allDay, &ev.TZName, &ev.RRuleText, &ev.Transparency, &ev.Visibility, &ev.ICSBlob, &attendees, &organizer); err != nil {
		return nil, err
	}
	ev.IsAllDay = allDay != 0
	if attendees != "" && attendees != "null" {
		if err := json.Unmarshal([]byte(attendees), &ev.Attendees); err != nil {
			return nil, err
		}
	}
	if organizer.Valid && organizer.String != "" && organizer.String != "null" {
		if err := json.Unmarshal([]byte(organizer.String), &ev.Organizer); err != nil {
			return nil, err
		}
	}
	return ev, nil
}
func getOverridesForRemoteSnapshot(ctx context.Context, tx *sql.Tx, id string) ([]EventOverride, error) {
	var count, total int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(CAST(ics_blob AS BLOB))),0) FROM event_recurrence_overrides WHERE event_id=?`, id).Scan(&count, &total); err != nil {
		return nil, err
	}
	if count > 256 || total > 1<<20 {
		return nil, errors.New("calendar override budget exceeded")
	}
	rows, err := tx.QueryContext(ctx, `SELECT event_id,recurrence_id_unix,ics_blob FROM event_recurrence_overrides WHERE event_id=? LIMIT 256`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []EventOverride{}
	for rows.Next() {
		var item EventOverride
		if err := rows.Scan(&item.EventID, &item.RecurrenceIDUnix, &item.ICSBlob); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
