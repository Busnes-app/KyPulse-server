package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/auditchain"
)

// anchorKey is the server_settings row holding the chain's count and head. It lives outside
// the log because hashes inside a table can never show that rows were deleted from its end.
const anchorKey = "audit_anchor"

// genesisHash is the predecessor of the first record, the anchor of an empty log.
const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// auditWriteBudget bounds one append. The audit write is not the caller's to cancel: a
// dropped connection must not lose the row for the action it already performed.
const auditWriteBudget = 10 * time.Second

// dbtx is what append and resume run their statements on: the pool at open, the caller's
// transaction otherwise. SQLite has one connection, so a read on the pool while a
// transaction is open would wait for itself.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type auditStore struct {
	store *SQLStore
	key   []byte

	mu    sync.Mutex        // serialises appends and guards chain
	chain *auditchain.Chain // nil until resumed, and again after a write the store could not confirm
}

// fieldsOf is the record content the chain authenticates. The order is the chain format:
// changing it invalidates every stored digest.
func fieldsOf(r *AuditRecord) []string {
	return []string{r.CreatedAt.UTC().Format(time.RFC3339Nano), r.UserID, r.Action, r.Resource, r.Details, r.IPAddress}
}

// stamp fixes the record time to what both dialects store, so a re-read reproduces the digest.
func stamp(r *AuditRecord) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	r.CreatedAt = r.CreatedAt.UTC().Truncate(time.Microsecond)
}

// open places the chain at start: a new log, a legacy log keyed once, or the stored tail.
func (a *auditStore) open(ctx context.Context) error {
	if len(a.key) < 32 {
		return fmt.Errorf("audit: chain key is %d bytes, want 32", len(a.key))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.resume(ctx, a.store.db, true)
	return err
}

// forget drops the in-memory chain after a write the store did not confirm; the next append
// re-reads the tail from disk instead of chaining onto a record that may not exist.
func (a *auditStore) forget() {
	a.mu.Lock()
	a.chain = nil
	a.mu.Unlock()
}

func (a *auditStore) loadAnchor(ctx context.Context, q dbtx) (auditchain.Anchor, bool, error) {
	var raw string
	err := q.QueryRowContext(ctx, a.store.rebind("SELECT value FROM server_settings WHERE key = ?"), anchorKey).Scan(&raw)
	if errorsIs(err, sql.ErrNoRows) {
		return auditchain.Anchor{}, false, nil
	}
	if err != nil {
		return auditchain.Anchor{}, false, err
	}
	var anchor auditchain.Anchor
	if err := json.Unmarshal([]byte(raw), &anchor); err != nil {
		return auditchain.Anchor{}, false, fmt.Errorf("%w: anchor row does not decode", ErrAuditUnplaceable)
	}
	return anchor, true, nil
}

func (a *auditStore) writeAnchor(ctx context.Context, q dbtx, anchor auditchain.Anchor) error {
	raw, _ := json.Marshal(anchor)
	_, err := q.ExecContext(ctx, a.store.rebind(`
INSERT INTO server_settings (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`), anchorKey, string(raw), time.Now().UTC())
	return err
}

// tail reads the highest-sequence record as a chain record.
func (a *auditStore) tail(ctx context.Context, q dbtx) (auditchain.Record, error) {
	var r AuditRecord
	var prev string
	err := q.QueryRowContext(ctx, `SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records WHERE seq IS NOT NULL ORDER BY seq DESC LIMIT 1`).Scan(&r.Seq, &prev, &r.Hash, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt)
	if err != nil {
		return auditchain.Record{}, err
	}
	return auditchain.Record{Seq: r.Seq, Prev: prev, Hash: r.Hash, Fields: fieldsOf(&r)}, nil
}

// resume returns the chain, placing it from the store when it is not in memory. mu is held.
// pool says q is the connection pool, which keying a legacy log needs (it opens its own
// transaction); from inside a caller's transaction that would deadlock on SQLite.
func (a *auditStore) resume(ctx context.Context, q dbtx, pool bool) (*auditchain.Chain, error) {
	if a.chain != nil {
		return a.chain, nil
	}
	anchor, hasAnchor, err := a.loadAnchor(ctx, q)
	if err != nil {
		return nil, err
	}
	var total, unkeyed uint64
	if err := q.QueryRowContext(ctx, "SELECT COUNT(1), COUNT(1) - COUNT(seq) FROM audit_records").Scan(&total, &unkeyed); err != nil {
		return nil, err
	}
	switch {
	case total == 0 && (!hasAnchor || anchor.Count == 0):
		c, err := auditchain.New(a.key)
		if err != nil {
			return nil, err
		}
		a.chain = c
		return c, nil
	case total == 0:
		return nil, fmt.Errorf("%w: no audit records, but the anchor counts %d; the log was emptied", ErrAuditUnplaceable, anchor.Count)
	case !hasAnchor && unkeyed == total:
		if !pool {
			return nil, fmt.Errorf("%w: %d unkeyed records and no anchor appeared while running", ErrAuditUnplaceable, total)
		}
		return a.keyLegacy(ctx, q)
	case !hasAnchor:
		return nil, fmt.Errorf("%w: %d keyed audit records but no anchor row (%s); a truncated log cannot be told from an intact one, so this server will not start. Restore the row from backup, or move the records aside to begin a new chain and keep the old ones for the auditor", ErrAuditUnplaceable, total, anchorKey)
	case unkeyed > 0:
		return nil, fmt.Errorf("%w: %d audit records carry no digest although the chain is anchored", ErrAuditUnplaceable, unkeyed)
	}
	last, err := a.tail(ctx, q)
	if err != nil {
		return nil, err
	}
	c, err := auditchain.Resume(a.key, last, anchor)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuditUnplaceable, err)
	}
	a.chain = c
	return c, nil
}

// keyLegacy digests a log written before the chain, in id order, and anchors it. It runs
// once, on the first start after the migration; every later row is chained at write time.
// q must be the pool: the rows are read to completion before the write transaction begins,
// which is what SQLite's single connection requires.
func (a *auditStore) keyLegacy(ctx context.Context, q dbtx) (*auditchain.Chain, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, user_id, action, resource, details, ip_address, created_at FROM audit_records ORDER BY id")
	if err != nil {
		return nil, err
	}
	var ids []int64
	var tuples [][]string
	for rows.Next() {
		var id int64
		var r AuditRecord
		if err := rows.Scan(&id, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		stamp(&r)
		ids = append(ids, id)
		tuples = append(tuples, fieldsOf(&r))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records, anchor, err := auditchain.Replay(a.key, tuples)
	if err != nil {
		return nil, err
	}
	tx, err := a.store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for i, rec := range records {
		if _, err := tx.ExecContext(ctx, a.store.rebind(`UPDATE audit_records SET seq = ?, prev_hash = ?, hash = ?, created_at = ? WHERE id = ?`),
			rec.Seq, rec.Prev, rec.Hash, mustTime(rec.Fields[0]), ids[i]); err != nil {
			return nil, err
		}
	}
	if err := a.writeAnchor(ctx, tx, anchor); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c, err := auditchain.Resume(a.key, records[len(records)-1], anchor)
	if err != nil {
		return nil, err
	}
	a.chain = c
	return c, nil
}

// mustTime parses the RFC3339Nano stamp fieldsOf produced; it cannot fail for our own output.
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic("audit: " + err.Error())
	}
	return t
}

// append chains r and writes it and the anchor on tx. The caller commits; if that commit
// fails it must call forget, because the chain has advanced past what the store holds.
func (a *auditStore) append(ctx context.Context, tx *sql.Tx, r *AuditRecord) error {
	stamp(r)
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := a.resume(ctx, tx, false)
	if err != nil {
		return err
	}
	rec, err := c.Append(ctx, func(rec auditchain.Record, anchor auditchain.Anchor) error {
		if _, err := tx.ExecContext(ctx, a.store.rebind(`
INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at, seq, prev_hash, hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`), r.UserID, r.Action, r.Resource, r.Details, r.IPAddress, r.CreatedAt, rec.Seq, rec.Prev, rec.Hash); err != nil {
			return err
		}
		return a.writeAnchor(ctx, tx, anchor)
	}, fieldsOf(r)...)
	if err != nil {
		a.chain = nil
		return err
	}
	r.Seq, r.Hash = rec.Seq, rec.Hash
	return nil
}

func (a *auditStore) LogAudit(ctx context.Context, r *AuditRecord) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditWriteBudget)
	defer cancel()
	tx, err := a.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := a.append(ctx, tx, r); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		a.forget()
		return err
	}
	return nil
}

func (a *auditStore) Ready(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.resume(ctx, a.store.db, true)
	return err
}

func (a *auditStore) VerifyChain(ctx context.Context) (ChainStatus, error) {
	anchor, ok, err := a.loadAnchor(ctx, a.store.db)
	if err != nil {
		return ChainStatus{}, err
	}
	if !ok {
		anchor = auditchain.Anchor{Count: 0, Hash: genesisHash}
	}
	rows, err := a.store.db.QueryContext(ctx, `SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records ORDER BY seq`)
	if err != nil {
		return ChainStatus{}, err
	}
	defer rows.Close()
	records := func(yield func(auditchain.Record, error) bool) {
		for rows.Next() {
			var r AuditRecord
			var seq sql.NullInt64
			var prev string
			if err := rows.Scan(&seq, &prev, &r.Hash, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt); err != nil {
				yield(auditchain.Record{}, err)
				return
			}
			if !seq.Valid {
				yield(auditchain.Record{}, errors.New("record written before the chain was keyed"))
				return
			}
			if !yield(auditchain.Record{Seq: uint64(seq.Int64), Prev: prev, Hash: r.Hash, Fields: fieldsOf(&r)}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(auditchain.Record{}, err)
		}
	}
	if err := auditchain.VerifyStream(a.key, records, anchor); err != nil {
		return ChainStatus{}, err
	}
	return ChainStatus{Count: anchor.Count, Head: anchor.Hash}, nil
}

func (a *auditStore) ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var count int
	if err := a.store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_records").Scan(&count); err != nil {
		return nil, 0, err
	}
	q := a.store.rebind(`
SELECT id, user_id, action, resource, details, ip_address, created_at, seq, hash
FROM audit_records
ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?
`)
	rows, err := a.store.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var records []*AuditRecord
	for rows.Next() {
		var r AuditRecord
		var seq sql.NullInt64
		if err := rows.Scan(&r.ID, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt, &seq, &r.Hash); err != nil {
			return nil, 0, err
		}
		if seq.Valid {
			r.Seq = uint64(seq.Int64)
		}
		records = append(records, &r)
	}
	return records, count, rows.Err()
}
