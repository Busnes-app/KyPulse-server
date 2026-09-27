package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ---------------------------------------------------------------------
// Target Store
// ---------------------------------------------------------------------

type targetStore struct {
	store *SQLStore
}

func (t *targetStore) CreateTarget(ctx context.Context, tg *Target) error {
	now := time.Now().UTC()
	if tg.State == "" {
		tg.State = "pending"
	}
	if tg.StateSince.IsZero() {
		tg.StateSince = now
	}
	if tg.IntervalSec == 0 {
		tg.IntervalSec = 30
	}
	tg.CreatedAt = now
	tg.UpdatedAt = now

	q := t.store.rebind(`
INSERT INTO targets (
    id, name, url, interval_sec, enabled, container, state, state_since, cause,
    track_json, last_result, last_polled_at, last_latency_ms, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	var lastPolled sql.NullTime
	if tg.LastPolledAt != nil {
		lastPolled = sql.NullTime{Time: *tg.LastPolledAt, Valid: true}
	}
	_, err := t.store.db.ExecContext(ctx, q,
		tg.ID, tg.Name, tg.URL, tg.IntervalSec, tg.Enabled, tg.Container, tg.State, tg.StateSince, tg.Cause,
		tg.TrackJSON, tg.LastResult, lastPolled, tg.LastLatencyMS, tg.CreatedAt, tg.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (t *targetStore) scanTarget(row interface{ Scan(...any) error }) (*Target, error) {
	var tg Target
	var lastPolled sql.NullTime
	err := row.Scan(
		&tg.ID, &tg.Name, &tg.URL, &tg.IntervalSec, &tg.Enabled, &tg.Container, &tg.State, &tg.StateSince, &tg.Cause,
		&tg.TrackJSON, &tg.LastResult, &lastPolled, &tg.LastLatencyMS, &tg.CreatedAt, &tg.UpdatedAt,
	)
	if err != nil {
		if errorsIs(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lastPolled.Valid {
		tg.LastPolledAt = &lastPolled.Time
	}
	return &tg, nil
}

const targetColumns = `id, name, url, interval_sec, enabled, container, state, state_since, cause,
       track_json, last_result, last_polled_at, last_latency_ms, created_at, updated_at`

func (t *targetStore) GetTarget(ctx context.Context, id string) (*Target, error) {
	q := t.store.rebind(`SELECT ` + targetColumns + ` FROM targets WHERE id = ?`)
	return t.scanTarget(t.store.db.QueryRowContext(ctx, q, id))
}

func (t *targetStore) ListTargets(ctx context.Context) ([]*Target, error) {
	q := `SELECT ` + targetColumns + ` FROM targets ORDER BY name`
	rows, err := t.store.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []*Target
	for rows.Next() {
		tg, err := t.scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, tg)
	}
	return targets, rows.Err()
}

func (t *targetStore) UpdateTarget(ctx context.Context, tg *Target) error {
	tg.UpdatedAt = time.Now().UTC()
	q := t.store.rebind(`UPDATE targets SET name = ?, url = ?, interval_sec = ?, enabled = ?, container = ?, updated_at = ? WHERE id = ?`)
	res, err := t.store.db.ExecContext(ctx, q, tg.Name, tg.URL, tg.IntervalSec, tg.Enabled, tg.Container, tg.UpdatedAt, tg.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyExists
		}
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *targetStore) DeleteTarget(ctx context.Context, id string) error {
	q := t.store.rebind("DELETE FROM targets WHERE id = ?")
	res, err := t.store.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *targetStore) RecordPoll(ctx context.Context, id string, u PollUpdate) error {
	q := t.store.rebind(`
UPDATE targets SET last_polled_at = ?, last_latency_ms = ?, last_result = ?, track_json = ?,
    state = ?, state_since = ?, cause = ?, updated_at = ?
WHERE id = ?
`)
	res, err := t.store.db.ExecContext(ctx, q, u.PolledAt, u.LatencyMS, u.LastResult, u.TrackJSON,
		u.State, u.StateSince, u.Cause, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *targetStore) SetTrack(ctx context.Context, id string, trackJSON string) error {
	q := t.store.rebind("UPDATE targets SET track_json = ?, updated_at = ? WHERE id = ?")
	res, err := t.store.db.ExecContext(ctx, q, trackJSON, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *targetStore) RecordEvent(ctx context.Context, e *TargetEvent) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	q := t.store.rebind(`
INSERT INTO target_events (target_id, at, from_state, to_state, cause, reminder, notified, notify_error)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`)
	if t.store.driver == "postgres" {
		var id int64
		err := t.store.db.QueryRowContext(ctx, q+" RETURNING id", e.TargetID, e.At, e.FromState, e.ToState, e.Cause, e.Reminder, e.Notified, e.NotifyError).Scan(&id)
		if err != nil {
			return err
		}
		e.ID = id
		return nil
	}
	res, err := t.store.db.ExecContext(ctx, q, e.TargetID, e.At, e.FromState, e.ToState, e.Cause, e.Reminder, e.Notified, e.NotifyError)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	e.ID = id
	return nil
}

func (t *targetStore) SetEventNotified(ctx context.Context, id int64, notified bool, notifyError string) error {
	q := t.store.rebind("UPDATE target_events SET notified = ?, notify_error = ? WHERE id = ?")
	res, err := t.store.db.ExecContext(ctx, q, notified, notifyError, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *targetStore) ListEvents(ctx context.Context, targetID string, offset, limit int) ([]*TargetEvent, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := ""
	var whereArgs []any
	if targetID != "" {
		where = " WHERE target_id = ?"
		whereArgs = []any{targetID}
	}

	var total int
	countQ := t.store.rebind("SELECT COUNT(1) FROM target_events" + where)
	if err := t.store.db.QueryRowContext(ctx, countQ, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQ := t.store.rebind(`
SELECT id, target_id, at, from_state, to_state, cause, reminder, notified, notify_error
FROM target_events` + where + `
ORDER BY at DESC, id DESC LIMIT ? OFFSET ?`)
	listArgs := append(append([]any{}, whereArgs...), limit, offset)

	rows, err := t.store.db.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []*TargetEvent
	for rows.Next() {
		var e TargetEvent
		if err := rows.Scan(&e.ID, &e.TargetID, &e.At, &e.FromState, &e.ToState, &e.Cause, &e.Reminder, &e.Notified, &e.NotifyError); err != nil {
			return nil, 0, err
		}
		events = append(events, &e)
	}
	return events, total, rows.Err()
}
