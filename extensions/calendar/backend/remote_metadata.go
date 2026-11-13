package backend

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) remoteSnapshot(ctx context.Context) (*sql.Tx, context.CancelFunc, error) {
	if ctx == nil || s == nil {
		return nil, func() {}, errors.New("Calendar store unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	return tx, cancel, nil
}

// RemoteSources bounds helper metadata before loading string payloads.
func (s *Store) RemoteSources(ctx context.Context) ([]Source, error) {
	tx, cancel, err := s.remoteSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback() }()
	var count, total int64
	err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(id AS BLOB))+length(CAST(type AS BLOB))+length(CAST(name AS BLOB))+length(CAST(url AS BLOB))+length(CAST(username AS BLOB))+length(CAST(coalesce(last_error,'') AS BLOB))+length(CAST(coalesce(account_id,'') AS BLOB))+length(CAST(coalesce(organizer_identities,'') AS BLOB))+length(CAST(coalesce(itip_mode,'') AS BLOB))),0) FROM calendar_sources`).Scan(&count, &total)
	if err != nil {
		return nil, err
	}
	if count > 128 || total > 1<<20 {
		return nil, errors.New("Calendar source metadata exceeds read budget")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,type,name,url,username,sync_interval_min,coalesce(last_synced_at,0),coalesce(last_error,''),coalesce(last_error_at,0),coalesce(account_id,''),enabled,writable,created_at,coalesce(itip_mode,'server'),coalesce(organizer_identities,'[]') FROM calendar_sources ORDER BY created_at LIMIT 128`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var item Source
		var enabled, writable int
		var identities string
		if err := rows.Scan(&item.ID, &item.Type, &item.Name, &item.URL, &item.Username, &item.SyncIntervalMin, &item.LastSyncedAt, &item.LastError, &item.LastErrorAt, &item.AccountID, &enabled, &writable, &item.CreatedAt, &item.ITIPMode, &identities); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		item.Writable = writable != 0
		item.OrganizerIdentities = unmarshalOrganizerIdentities(identities)
		out = append(out, item)
	}
	return out, rows.Err()
}

// RemoteCalendars reads one source in a consistent, bounded snapshot.
func (s *Store) RemoteCalendars(ctx context.Context, sourceID string) ([]Calendar, error) {
	if sourceID == "" || len(sourceID) > 200 {
		return nil, errors.New("invalid Calendar source")
	}
	tx, cancel, err := s.remoteSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback() }()
	var count, total int64
	err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(id AS BLOB))+length(CAST(source_id AS BLOB))+length(CAST(url AS BLOB))+length(CAST(display_name AS BLOB))+length(CAST(coalesce(description,'') AS BLOB))+length(CAST(coalesce(color,'') AS BLOB))+length(CAST(coalesce(ctag,'') AS BLOB))),0) FROM calendars WHERE source_id=?`, sourceID).Scan(&count, &total)
	if err != nil {
		return nil, err
	}
	if count > 256 || total > 1<<20 {
		return nil, errors.New("Calendar metadata exceeds read budget")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,source_id,url,display_name,coalesce(description,''),coalesce(color,''),visible,writable,coalesce(ctag,''),coalesce(last_synced_at,0),created_at FROM calendars WHERE source_id=? ORDER BY display_name LIMIT 256`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Calendar{}
	for rows.Next() {
		var item Calendar
		var visible, writable int
		if err := rows.Scan(&item.ID, &item.SourceID, &item.URL, &item.DisplayName, &item.Description, &item.Color, &visible, &writable, &item.Ctag, &item.LastSyncedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Visible = visible != 0
		item.Writable = writable != 0
		out = append(out, item)
	}
	return out, rows.Err()
}

// RemoteEvent returns a bounded event without trusting its HTML. Desktop renderers
// must apply their normal HTML sanitizer before displaying rich content.
func (s *Store) RemoteEvent(ctx context.Context, id string) (*Event, error) {
	if id == "" || len(id) > 200 {
		return nil, errors.New("invalid Calendar event")
	}
	tx, cancel, err := s.remoteSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback() }()
	var size int64
	err = tx.QueryRowContext(ctx, `SELECT length(CAST(coalesce(ics_blob,'') AS BLOB))+length(CAST(coalesce(description,'') AS BLOB))+length(CAST(coalesce(summary,'') AS BLOB))+length(CAST(coalesce(location,'') AS BLOB))+length(CAST(coalesce(attendees_json,'') AS BLOB))+length(CAST(coalesce(organizer_json,'') AS BLOB))+length(CAST(coalesce(uid,'') AS BLOB))+length(CAST(coalesce(href,'') AS BLOB))+length(CAST(coalesce(etag,'') AS BLOB))+length(CAST(coalesce(provider_event_id,'') AS BLOB))+length(CAST(coalesce(rrule_text,'') AS BLOB))+length(CAST(coalesce(tz_name,'') AS BLOB))+length(CAST(coalesce(transparency,'') AS BLOB))+length(CAST(coalesce(visibility,'') AS BLOB)) FROM events WHERE id=?`, id).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if size > 1<<20 {
		return nil, errors.New("Calendar event exceeds read budget")
	}
	event, err := getEventForRemoteSnapshot(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	event.Description = unescapeICalText(event.Description)
	return event, nil
}
