package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type LogLine struct {
	ID         int64     `json:"id"`
	Time       time.Time `json:"time"`
	ReceivedAt time.Time `json:"received_at"`
	SourceID   string    `json:"source_id"`
	Source     string    `json:"source"`
	TargetID   string    `json:"target_id"`
	App        string    `json:"app"`
	Level      string    `json:"level"`
	Event      string    `json:"event"`
	Message    string    `json:"message"`
	Raw        string    `json:"raw"`
	Truncated  bool      `json:"truncated"`
	Bytes      int64     `json:"-"`
}

type Activity struct {
	ID          int64     `json:"id"`
	Time        time.Time `json:"time"`
	ReceivedAt  time.Time `json:"received_at"`
	SourceID    string    `json:"source_id"`
	TargetID    string    `json:"target_id"`
	App         string    `json:"app"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	Target      string    `json:"target"`
	Outcome     string    `json:"outcome"`
	IP          string    `json:"ip"`
	ExternalKey string    `json:"-"`
	Bytes       int64     `json:"-"`
}

type LogFilter struct {
	TargetID, App, Level, Text string
	From, To                   time.Time
	BeforeID                   int64
	Limit                      int
}
type ActivityFilter struct {
	TargetID, App, Actor, Outcome string
	From, To                      time.Time
	BeforeID                      int64
	Limit                         int
}
type LogBatch struct {
	Logs     []LogLine
	Activity []Activity
}
type LogStore interface {
	Append(ctx context.Context, sourceID string, batch LogBatch, maxBytes int64) error
	List(ctx context.Context, f LogFilter) ([]LogLine, error)
	ListActivity(ctx context.Context, f ActivityFilter) ([]Activity, error)
	Prune(ctx context.Context, now time.Time, maxBytes int64) error
}
type logStore struct{ store *SQLStore }

const rowAllowance int64 = 64

func logBytes(l LogLine) int64 {
	return rowAllowance + int64(len(l.SourceID)+len(l.Source)+len(l.TargetID)+len(l.App)+len(l.Level)+len(l.Event)+len(l.Message)+len(l.Raw))
}
func activityBytes(a Activity) int64 {
	return rowAllowance + int64(len(a.SourceID)+len(a.TargetID)+len(a.App)+len(a.Actor)+len(a.Action)+len(a.Target)+len(a.Outcome)+len(a.IP)+len(a.ExternalKey))
}
func nullableID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (l *logStore) lock(ctx context.Context, tx *sql.Tx) error {
	// SQLite acquires its writer lock; PostgreSQL holds this row lock through commit.
	_, err := tx.ExecContext(ctx, "UPDATE log_usage SET bytes=bytes WHERE id=1")
	return err
}
func (l *logStore) Append(ctx context.Context, sourceID string, batch LogBatch, maxBytes int64) error {
	tx, err := l.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = l.lock(ctx, tx); err != nil {
		return err
	}
	// Task 2 introduces the source table and active-source check in this transaction.
	if sourceID != "" {
		return ErrNotFound
	}
	var added int64
	for i := range batch.Logs {
		r := &batch.Logs[i]
		r.SourceID = sourceID
		r.Bytes = logBytes(*r)
		q := l.store.rebind(`INSERT INTO log_lines(time,received_at,source_id,source,target_id,app,level,event,message,raw,truncated,bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
		args := []any{r.Time, r.ReceivedAt, r.SourceID, r.Source, nullableID(r.TargetID), r.App, r.Level, r.Event, r.Message, r.Raw, r.Truncated, r.Bytes}
		if l.store.driver == "postgres" {
			err = tx.QueryRowContext(ctx, q+" RETURNING id", args...).Scan(&r.ID)
		} else {
			var res sql.Result
			res, err = tx.ExecContext(ctx, q, args...)
			if err == nil {
				r.ID, err = res.LastInsertId()
			}
		}
		if err != nil {
			return err
		}
		added += r.Bytes
	}
	for i := range batch.Activity {
		r := &batch.Activity[i]
		r.SourceID = sourceID
		r.Bytes = activityBytes(*r)
		if r.ExternalKey != "" {
			var old Activity
			var target sql.NullString
			q := l.store.rebind(`SELECT time,received_at,source_id,target_id,app,actor,action,target,outcome,ip,bytes FROM activity WHERE external_key=?`)
			check := tx.QueryRowContext(ctx, q, r.ExternalKey).Scan(&old.Time, &old.ReceivedAt, &old.SourceID, &target, &old.App, &old.Actor, &old.Action, &old.Target, &old.Outcome, &old.IP, &old.Bytes)
			if check == nil {
				old.TargetID = target.String
				if old.Time.Truncate(time.Microsecond).Equal(r.Time.Truncate(time.Microsecond)) && old.ReceivedAt.Truncate(time.Microsecond).Equal(r.ReceivedAt.Truncate(time.Microsecond)) && old.SourceID == r.SourceID && old.TargetID == r.TargetID && old.App == r.App && old.Actor == r.Actor && old.Action == r.Action && old.Target == r.Target && old.Outcome == r.Outcome && old.IP == r.IP && old.Bytes == r.Bytes {
					continue
				}
				return ErrAlreadyExists
			}
			if !errors.Is(check, sql.ErrNoRows) {
				return check
			}
		}
		q := l.store.rebind(`INSERT INTO activity(time,received_at,source_id,target_id,app,actor,action,target,outcome,ip,external_key,bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
		args := []any{r.Time, r.ReceivedAt, r.SourceID, nullableID(r.TargetID), r.App, r.Actor, r.Action, r.Target, r.Outcome, r.IP, r.ExternalKey, r.Bytes}
		if l.store.driver == "postgres" {
			err = tx.QueryRowContext(ctx, q+" RETURNING id", args...).Scan(&r.ID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
		} else {
			var res sql.Result
			res, err = tx.ExecContext(ctx, q, args...)
			if err == nil {
				var n int64
				n, err = res.RowsAffected()
				if err == nil && n == 0 {
					continue
				}
				if err == nil {
					r.ID, err = res.LastInsertId()
				}
			}
		}
		if err != nil {
			return err
		}
		added += r.Bytes
	}
	if _, err = tx.ExecContext(ctx, l.store.rebind("UPDATE log_usage SET bytes=bytes+? WHERE id=1"), added); err != nil {
		return err
	}
	if err = l.pruneTx(ctx, tx, time.Time{}, maxBytes); err != nil {
		return err
	}
	return tx.Commit()
}
func (l *logStore) Prune(ctx context.Context, now time.Time, maxBytes int64) error {
	tx, err := l.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = l.lock(ctx, tx); err != nil {
		return err
	}
	if err = l.pruneTx(ctx, tx, now, maxBytes); err != nil {
		return err
	}
	return tx.Commit()
}
func (l *logStore) pruneTx(ctx context.Context, tx *sql.Tx, now time.Time, maxBytes int64) error {
	if !now.IsZero() {
		cutoff := now.Add(-7 * 24 * time.Hour)
		for _, table := range []string{"log_lines", "activity"} {
			if _, err := tx.ExecContext(ctx, l.store.rebind("DELETE FROM "+table+" WHERE received_at < ?"), cutoff); err != nil {
				return err
			}
		}
	}
	var total int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT SUM(bytes) FROM log_lines),0)+COALESCE((SELECT SUM(bytes) FROM activity),0)").Scan(&total); err != nil {
		return err
	}
	if maxBytes >= 0 && total > maxBytes {
		rows, err := tx.QueryContext(ctx, `SELECT kind,id,bytes FROM (SELECT 0 AS kind,id,received_at,bytes FROM log_lines UNION ALL SELECT 1 AS kind,id,received_at,bytes FROM activity) AS all_rows ORDER BY received_at,id,kind`)
		if err != nil {
			return err
		}
		type victim struct {
			kind      int
			id, bytes int64
		}
		var victims []victim
		for rows.Next() && total > maxBytes {
			var v victim
			if err = rows.Scan(&v.kind, &v.id, &v.bytes); err != nil {
				break
			}
			victims = append(victims, v)
			total -= v.bytes
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, v := range victims {
			table := "log_lines"
			if v.kind == 1 {
				table = "activity"
			}
			if _, err = tx.ExecContext(ctx, l.store.rebind("DELETE FROM "+table+" WHERE id=?"), v.id); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, l.store.rebind("UPDATE log_usage SET bytes=? WHERE id=1"), total)
	return err
}
func pageLimit(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 200 {
		return 200
	}
	return n
}
func escapedLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}
func addFilter(parts *[]string, args *[]any, col string, val any) {
	*parts = append(*parts, col+" = ?")
	*args = append(*args, val)
}
func (l *logStore) List(ctx context.Context, f LogFilter) ([]LogLine, error) {
	var where []string
	var args []any
	if f.TargetID != "" {
		addFilter(&where, &args, "target_id", f.TargetID)
	}
	if f.App != "" {
		addFilter(&where, &args, "app", f.App)
	}
	if f.Level != "" {
		addFilter(&where, &args, "level", f.Level)
	}
	if f.Text != "" {
		where = append(where, `(message LIKE ? ESCAPE '\' OR raw LIKE ? ESCAPE '\')`)
		v := "%" + escapedLike(f.Text) + "%"
		args = append(args, v, v)
	}
	addRange(&where, &args, f.From, f.To, f.BeforeID)
	q := `SELECT id,time,received_at,source_id,source,target_id,app,level,event,message,raw,truncated,bytes FROM log_lines` + whereSQL(where) + ` ORDER BY id DESC LIMIT ?`
	args = append(args, pageLimit(f.Limit))
	rows, err := l.store.db.QueryContext(ctx, l.store.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogLine{}
	for rows.Next() {
		var v LogLine
		var target sql.NullString
		if err = rows.Scan(&v.ID, &v.Time, &v.ReceivedAt, &v.SourceID, &v.Source, &target, &v.App, &v.Level, &v.Event, &v.Message, &v.Raw, &v.Truncated, &v.Bytes); err != nil {
			return nil, err
		}
		v.TargetID = target.String
		out = append(out, v)
	}
	return out, rows.Err()
}
func (l *logStore) ListActivity(ctx context.Context, f ActivityFilter) ([]Activity, error) {
	var where []string
	var args []any
	if f.TargetID != "" {
		addFilter(&where, &args, "target_id", f.TargetID)
	}
	if f.App != "" {
		addFilter(&where, &args, "app", f.App)
	}
	if f.Actor != "" {
		addFilter(&where, &args, "actor", f.Actor)
	}
	if f.Outcome != "" {
		addFilter(&where, &args, "outcome", f.Outcome)
	}
	addRange(&where, &args, f.From, f.To, f.BeforeID)
	q := `SELECT id,time,received_at,source_id,target_id,app,actor,action,target,outcome,ip,external_key,bytes FROM activity` + whereSQL(where) + ` ORDER BY id DESC LIMIT ?`
	args = append(args, pageLimit(f.Limit))
	rows, err := l.store.db.QueryContext(ctx, l.store.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var v Activity
		var target sql.NullString
		if err = rows.Scan(&v.ID, &v.Time, &v.ReceivedAt, &v.SourceID, &target, &v.App, &v.Actor, &v.Action, &v.Target, &v.Outcome, &v.IP, &v.ExternalKey, &v.Bytes); err != nil {
			return nil, err
		}
		v.TargetID = target.String
		out = append(out, v)
	}
	return out, rows.Err()
}
func addRange(where *[]string, args *[]any, from, to time.Time, before int64) {
	if !from.IsZero() {
		*where = append(*where, "time >= ?")
		*args = append(*args, from)
	}
	if !to.IsZero() {
		*where = append(*where, "time <= ?")
		*args = append(*args, to)
	}
	if before > 0 {
		*where = append(*where, "id < ?")
		*args = append(*args, before)
	}
}
func whereSQL(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(parts, " AND ")
}

var _ LogStore = (*logStore)(nil)
