package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
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

type RetainedRow struct {
	Kind       string
	ID         int64
	ReceivedAt time.Time
	Bytes      int64
}

// Evict chooses expired rows first, then the oldest rows needed to fit the budget.
func Evict(rows []RetainedRow, now time.Time, maxBytes int64) []RetainedRow {
	ordered := append([]RetainedRow(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !a.ReceivedAt.Equal(b.ReceivedAt) {
			return a.ReceivedAt.Before(b.ReceivedAt)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	var total int64
	for _, r := range ordered {
		total += r.Bytes
	}
	var out []RetainedRow
	for _, r := range ordered {
		if !now.IsZero() && r.ReceivedAt.Before(now.Add(-7*24*time.Hour)) {
			out = append(out, r)
			total -= r.Bytes
		}
	}
	for _, r := range ordered {
		if total <= maxBytes {
			break
		}
		if !now.IsZero() && r.ReceivedAt.Before(now.Add(-7*24*time.Hour)) {
			continue
		}
		out = append(out, r)
		total -= r.Bytes
	}
	return out
}

type LogCursor struct{ Key, Value string }

var ErrCursorConflict = errors.New("log cursor changed")

type LogStore interface {
	Cursor(ctx context.Context, key string) (string, error)
	AppendImported(ctx context.Context, batch LogBatch, previous, next LogCursor, maxBytes int64) error
	Append(ctx context.Context, sourceID string, batch LogBatch, maxBytes int64) error
	List(ctx context.Context, f LogFilter) ([]LogLine, error)
	ListActivity(ctx context.Context, f ActivityFilter) ([]Activity, error)
	ActivityBursts(ctx context.Context, f ActivityFilter, rule ActivityBurstRule) ([]ActivityBurst, error)
	Prune(ctx context.Context, now time.Time, maxBytes int64) error
}
type logStore struct{ store *SQLStore }

// Fixed space for row IDs, timestamps, and flags.
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
func (l *logStore) lock(ctx context.Context, tx *sql.Tx) (int64, error) {
	// SQLite acquires its writer lock; PostgreSQL holds this row lock through commit.
	if _, err := tx.ExecContext(ctx, "UPDATE log_usage SET bytes=bytes WHERE id=1"); err != nil {
		return 0, err
	}
	var total int64
	err := tx.QueryRowContext(ctx, "SELECT bytes FROM log_usage WHERE id=1").Scan(&total)
	return total, err
}
func (l *logStore) Cursor(ctx context.Context, key string) (string, error) {
	var value string
	err := l.store.db.QueryRowContext(ctx, l.store.rebind("SELECT value FROM log_cursors WHERE key=?"), key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}
func (l *logStore) AppendImported(ctx context.Context, batch LogBatch, previous, next LogCursor, maxBytes int64) error {
	if previous.Key == "" || previous.Key != next.Key {
		return ErrCursorConflict
	}
	return l.append(ctx, "", batch, &previous, &next, maxBytes)
}
func (l *logStore) Append(ctx context.Context, sourceID string, batch LogBatch, maxBytes int64) error {
	return l.append(ctx, sourceID, batch, nil, nil, maxBytes)
}
func (l *logStore) append(ctx context.Context, sourceID string, batch LogBatch, previous, next *LogCursor, maxBytes int64) error {
	tx, err := l.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	total, err := l.lock(ctx, tx)
	if err != nil {
		return err
	}
	if previous != nil {
		var value string
		err = tx.QueryRowContext(ctx, l.store.rebind("SELECT value FROM log_cursors WHERE key=?"), previous.Key).Scan(&value)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if value != previous.Value {
			return ErrCursorConflict
		}
		if _, err = tx.ExecContext(ctx, l.store.rebind("INSERT INTO log_cursors(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value"), next.Key, next.Value); err != nil {
			return err
		}
	}
	if sourceID != "" {
		var source LogSource
		var target sql.NullString
		err = tx.QueryRowContext(ctx, l.store.rebind("SELECT name,target_id FROM log_sources WHERE id=? AND revoked_at IS NULL"), sourceID).Scan(&source.Name, &target)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		source.TargetID = target.String
		for i := range batch.Logs {
			batch.Logs[i].Source = source.Name
			batch.Logs[i].TargetID = source.TargetID
		}
		for i := range batch.Activity {
			batch.Activity[i].TargetID = source.TargetID
		}
	}
	var added int64
	for i := range batch.Logs {
		r := &batch.Logs[i]
		r.Time = r.Time.UTC()
		r.ReceivedAt = r.ReceivedAt.UTC()
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
		r.Time = r.Time.UTC()
		r.ReceivedAt = r.ReceivedAt.UTC()
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
	if err = l.pruneTx(ctx, tx, time.Time{}, maxBytes, total+added); err != nil {
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
	total, err := l.lock(ctx, tx)
	if err != nil {
		return err
	}
	if err = l.pruneTx(ctx, tx, now, maxBytes, total); err != nil {
		return err
	}
	return tx.Commit()
}
func (l *logStore) pruneTx(ctx context.Context, tx *sql.Tx, now time.Time, maxBytes, total int64) error {
	if !now.IsZero() {
		cutoff := now.UTC().Add(-7 * 24 * time.Hour)
		for _, table := range []string{"log_lines", "activity"} {
			var expired int64
			if err := tx.QueryRowContext(ctx, l.store.rebind("SELECT COALESCE(SUM(bytes),0) FROM "+table+" WHERE received_at < ?"), cutoff).Scan(&expired); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, l.store.rebind("DELETE FROM "+table+" WHERE received_at < ?"), cutoff); err != nil {
				return err
			}
			total -= expired
		}
	}
	for maxBytes >= 0 && total > maxBytes {
		rows, err := tx.QueryContext(ctx, `SELECT kind,id,received_at,bytes FROM (SELECT 'log' AS kind,id,received_at,bytes FROM log_lines UNION ALL SELECT 'activity' AS kind,id,received_at,bytes FROM activity) AS all_rows ORDER BY received_at,kind,id LIMIT 256`)
		if err != nil {
			return err
		}
		var page []RetainedRow
		var pageBytes int64
		for rows.Next() {
			var v RetainedRow
			if err = rows.Scan(&v.Kind, &v.ID, &v.ReceivedAt, &v.Bytes); err != nil {
				break
			}
			page = append(page, v)
			pageBytes += v.Bytes
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return errors.New("log usage exceeds retained rows")
		}
		for _, v := range Evict(page, time.Time{}, maxBytes-(total-pageBytes)) {
			table := "log_lines"
			if v.Kind == "activity" {
				table = "activity"
			}
			if _, err = tx.ExecContext(ctx, l.store.rebind("DELETE FROM "+table+" WHERE id=?"), v.ID); err != nil {
				return err
			}
			total -= v.Bytes
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
	where, args := activityFilters(f, true)
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
		*args = append(*args, from.UTC())
	}
	if !to.IsZero() {
		*where = append(*where, "time <= ?")
		*args = append(*args, to.UTC())
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
