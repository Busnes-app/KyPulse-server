package store

import (
	"context"
	"strings"
	"time"
)

type ActivityBurstRule struct {
	Actions []string
	Outcome string
	Minimum int
	Window  time.Duration
}
type ActivityBurst struct {
	App   string    `json:"app"`
	Actor string    `json:"actor"`
	IP    string    `json:"ip"`
	Count int       `json:"count"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
}

// ActivityBursts selects the newest qualifying window per app/actor or app/IP.
// Window calculation stays in SQL; only 100 groups cross the storage boundary.
// Pagination affects timeline rows, never this retained-activity summary.
func (l *logStore) ActivityBursts(ctx context.Context, f ActivityFilter, rule ActivityBurstRule) ([]ActivityBurst, error) {
	out := []ActivityBurst{}
	if len(rule.Actions) == 0 {
		return out, nil
	}
	where, args := activityFilters(f, false)
	where = append(where, "action IN ("+strings.TrimSuffix(strings.Repeat("?,", len(rule.Actions)), ",")+")", "outcome = ?")
	for _, action := range rule.Actions {
		args = append(args, action)
	}
	args = append(args, rule.Outcome)
	// SQLite dates retain fractional seconds as text. Convert the integral seconds
	// and fractional part separately to avoid Julian-day floating-point boundary loss.
	stamp := `CAST(strftime('%s',substr(time,1,19)) AS INTEGER)*1000000 + CASE WHEN substr(time,20,1)='.' THEN CAST(ROUND(CAST('0.' || substr(time,21) AS REAL)*1000000) AS INTEGER) ELSE 0 END`
	if l.store.driver == "postgres" {
		stamp = `CAST(EXTRACT(EPOCH FROM time)*1000000 AS BIGINT)`
	}
	q := `WITH filtered AS (SELECT id,app,actor,ip,` + stamp + ` AS stamp FROM activity` + whereSQL(where) + `),
 identities AS (
 SELECT id,app,actor AS identity,'actor' AS kind,stamp FROM filtered WHERE actor <> ''
 UNION ALL SELECT id,app,ip AS identity,'ip' AS kind,stamp FROM filtered WHERE ip <> ''
 ), windows AS (
 SELECT *,COUNT(*) OVER w AS n,FIRST_VALUE(id) OVER w AS first_id
 FROM identities WINDOW w AS (PARTITION BY app,kind,identity ORDER BY stamp RANGE BETWEEN ? PRECEDING AND CURRENT ROW)
 ), ranked AS (
 SELECT *,ROW_NUMBER() OVER (PARTITION BY app,kind,identity ORDER BY stamp DESC,id DESC) AS rank
 FROM windows WHERE n >= ?
 ) SELECT r.app,CASE WHEN r.kind='actor' THEN r.identity ELSE '' END,
 CASE WHEN r.kind='ip' THEN r.identity ELSE '' END,r.n,a.time,b.time
 FROM ranked r JOIN activity a ON a.id=r.first_id JOIN activity b ON b.id=r.id
 WHERE r.rank=1 ORDER BY r.stamp DESC,r.app,r.kind,r.identity LIMIT 100`
	args = append(args, rule.Window.Microseconds(), rule.Minimum)
	rows, err := l.store.db.QueryContext(ctx, l.store.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v ActivityBurst
		if err := rows.Scan(&v.App, &v.Actor, &v.IP, &v.Count, &v.From, &v.To); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func activityFilters(f ActivityFilter, cursor bool) ([]string, []any) {
	var where []string
	var args []any
	for _, v := range []struct{ column, value string }{{"target_id", f.TargetID}, {"app", f.App}, {"actor", f.Actor}, {"outcome", f.Outcome}} {
		if v.value != "" {
			addFilter(&where, &args, v.column, v.value)
		}
	}
	before := int64(0)
	if cursor {
		before = f.BeforeID
	}
	addRange(&where, &args, f.From, f.To, before)
	return where, args
}
