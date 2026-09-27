package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/auditchain"
	"github.com/jackc/pgx/v5/pgconn"
)

// anchorKey is the server_settings row holding the chain's count and head. It lives outside
// the log because hashes inside a table can never show that rows were deleted from its end.
// It is in the same database as the log, so it catches a DELETE that leaves server_settings
// alone and a crash mid-write; it does not catch someone who can write both tables. The
// audit_chain_placed log line at every start and the audit-verify output carry count and
// head, which is the operator's copy outside the database.
const anchorKey = "audit_anchor"

// genesisHash is the predecessor of the first record, the anchor of an empty log.
const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// auditLockKey is the Postgres transaction-scoped advisory lock every append takes before
// reading the anchor, so appends from separate processes on one database run one at a time.
const auditLockKey int64 = 7345101

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

	placement ChainPlacement // set once by open, read-only after

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
// allowLegacy is true only when migration 6 was applied by this open.
func (a *auditStore) open(ctx context.Context, allowLegacy bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, mode, err := a.place(ctx, a.store.db, allowLegacy)
	if err != nil {
		return err
	}
	a.chain = c
	anchor := c.Anchor()
	a.placement = ChainPlacement{Mode: mode, Count: anchor.Count, Head: anchor.Hash}
	return nil
}

func (a *auditStore) Placement() ChainPlacement { return a.placement }

func decodeAnchor(raw string) (auditchain.Anchor, error) {
	var anchor auditchain.Anchor
	if err := json.Unmarshal([]byte(raw), &anchor); err != nil {
		return auditchain.Anchor{}, fmt.Errorf("%w: anchor row does not decode", ErrAuditUnplaceable)
	}
	return anchor, nil
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
	anchor, err := decodeAnchor(raw)
	return anchor, err == nil, err
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

// tail reads the record at seq as a chain record.
func (a *auditStore) tail(ctx context.Context, q dbtx, seq uint64) (auditchain.Record, error) {
	var r AuditRecord
	var prev string
	err := q.QueryRowContext(ctx, a.store.rebind(`SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records WHERE seq = ?`), seq).Scan(&r.Seq, &prev, &r.Hash, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt)
	if errorsIs(err, sql.ErrNoRows) {
		return auditchain.Record{}, fmt.Errorf("%w: the anchor names record %d, which is not in the log", ErrAuditUnplaceable, seq)
	}
	if err != nil {
		return auditchain.Record{}, err
	}
	return auditchain.Record{Seq: r.Seq, Prev: prev, Hash: r.Hash, Fields: fieldsOf(&r)}, nil
}

// place determines the chain from the store's current state and names how it found it
// ("new", "legacy_keyed", "resumed"). Anchor, counts and highest sequence come from one statement, so they are
// one snapshot; the tail is then read by the anchor's sequence, which rows appended since
// cannot change. It is a pure read except when allowLegacy and the log is all-legacy, in
// which case keying it is the one write place performs. Unlike resume, place never reads or
// writes a.chain, which is what lets Ready call it without mu.
//
// allowLegacy is only ever true from open, where q is the pool: keying opens its own
// transaction, which from inside a caller's transaction would deadlock on SQLite.
func (a *auditStore) place(ctx context.Context, q dbtx, allowLegacy bool) (*auditchain.Chain, string, error) {
	var raw sql.NullString
	var total, unkeyed, maxSeq uint64
	if err := q.QueryRowContext(ctx, a.store.rebind("SELECT (SELECT value FROM server_settings WHERE key = ?), COUNT(1), COUNT(1) - COUNT(seq), COALESCE(MAX(seq), 0) FROM audit_records"), anchorKey).Scan(&raw, &total, &unkeyed, &maxSeq); err != nil {
		return nil, "", err
	}
	var anchor auditchain.Anchor
	hasAnchor := raw.Valid
	if hasAnchor {
		var err error
		if anchor, err = decodeAnchor(raw.String); err != nil {
			return nil, "", err
		}
	}
	switch {
	case total == 0 && (!hasAnchor || anchor.Count == 0):
		c, err := auditchain.New(a.key)
		return c, "new", err
	case total == 0:
		return nil, "", fmt.Errorf("%w: no audit records, but the anchor counts %d; the log was emptied", ErrAuditUnplaceable, anchor.Count)
	case !hasAnchor && unkeyed == total:
		if !allowLegacy {
			return nil, "", fmt.Errorf("%w: %d audit records carry no digest and there is no anchor, but the chain was keyed before: the chain columns and the anchor were cleared. This server will not start. Restore the database from backup, or move the records aside to begin a new chain and keep the old ones for the auditor. If this follows a crash during the first start after the upgrade, run `DELETE FROM schema_migrations WHERE version = 6`, drop the index idx_audit_seq, then the columns seq, prev_hash and hash from audit_records, and start again; the keying reruns", ErrAuditUnplaceable, total)
		}
		c, err := a.keyLegacy(ctx, q)
		return c, "legacy_keyed", err
	case !hasAnchor:
		return nil, "", fmt.Errorf("%w: %d keyed audit records but no anchor row (%s); a truncated log cannot be told from an intact one, so this server will not start. Restore the row from backup, or move the records aside to begin a new chain and keep the old ones for the auditor", ErrAuditUnplaceable, total, anchorKey)
	case unkeyed > 0:
		return nil, "", fmt.Errorf("%w: %d audit records carry no digest although the chain is anchored", ErrAuditUnplaceable, unkeyed)
	case total != anchor.Count:
		return nil, "", fmt.Errorf("%w: %d audit records but the anchor counts %d", ErrAuditUnplaceable, total, anchor.Count)
	case maxSeq != anchor.Count:
		// Unique positive seqs with total == count == max are exactly 1..N.
		return nil, "", fmt.Errorf("%w: %d audit records but the highest sequence is %d while the anchor counts %d", ErrAuditUnplaceable, total, maxSeq, anchor.Count)
	}
	last, err := a.tail(ctx, q, anchor.Count)
	if err != nil {
		return nil, "", err
	}
	c, err := auditchain.Resume(a.key, last, anchor)
	if err != nil {
		return nil, "", fmt.Errorf("%w: the audit key (KYPULSE_AUDIT_KEY or <DataDir>/audit.key) is not the one that wrote this log, or the tail record was altered: %v. With the right key the server starts; without it, move the records and the anchor aside to begin a new chain and keep the old ones for the auditor", ErrAuditUnplaceable, err)
	}
	return c, "resumed", nil
}

// resume returns the chain, memoising it in a.chain once placed. The caller must hold mu:
// it reads and writes a.chain, which a concurrent append also touches.
func (a *auditStore) resume(ctx context.Context, q dbtx) (*auditchain.Chain, error) {
	if a.chain != nil {
		return a.chain, nil
	}
	c, _, err := a.place(ctx, q, false)
	if err != nil {
		return nil, err
	}
	a.chain = c
	return c, nil
}

// keyLegacy digests a log written before the chain, in id order, and anchors it. It runs
// once, in the open that applied migration 6; every later row is chained at write time.
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
	return auditchain.Resume(a.key, records[len(records)-1], anchor)
}

// mustTime parses the RFC3339Nano stamp fieldsOf produced; it cannot fail for our own output.
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic("audit: " + err.Error())
	}
	return t
}

// append chains r and writes it and the anchor on tx. On success it returns with mu still
// held and hands back finish, which the caller must call exactly once with the transaction's
// commit outcome: finish releases mu, and clears the chain first if the commit did not
// happen, so the next append re-reads the tail from disk instead of chaining onto a record
// that may not exist. Holding mu until finish — rather than releasing it here — is what stops
// a second append from chaining onto this one before it is durable: on Postgres, unlike
// SQLite, a second connection could otherwise start its own append while this transaction is
// still open. Another process on the same database may have appended since this one last
// did, so the stored anchor is compared with the chain's and the tail re-read on a mismatch
// or a missing anchor.
func (a *auditStore) append(ctx context.Context, tx *sql.Tx, r *AuditRecord) (finish func(committed bool), err error) {
	stamp(r)
	a.mu.Lock()
	c, err := a.follow(ctx, tx)
	if err != nil {
		a.chain = nil
		a.mu.Unlock()
		return nil, err
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
		a.mu.Unlock()
		return nil, err
	}
	r.Seq, r.Hash = rec.Seq, rec.Hash
	return func(committed bool) {
		if !committed {
			a.chain = nil
		}
		a.mu.Unlock()
	}, nil
}

// follow resumes the chain and re-places it on tx when the stored anchor has moved on or is
// gone; a missing anchor then refuses, rather than being re-created from memory.
func (a *auditStore) follow(ctx context.Context, tx *sql.Tx) (*auditchain.Chain, error) {
	// Serialise with other processes before reading the anchor. SQLite has no advisory lock:
	// a write statement, even one matching no row, takes the database write lock and waits
	// out busy_timeout, where a deferred transaction that read first fails with BUSY_SNAPSHOT.
	lock, args := "UPDATE server_settings SET key = key WHERE 0", []any(nil)
	if a.store.driver == "postgres" {
		lock, args = "SELECT pg_advisory_xact_lock($1)", []any{auditLockKey}
	}
	if _, err := tx.ExecContext(ctx, lock, args...); err != nil {
		return nil, err
	}
	c, err := a.resume(ctx, tx)
	if err != nil {
		return nil, err
	}
	stored, ok, err := a.loadAnchor(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !ok || stored != c.Anchor() {
		a.chain = nil
		return a.resume(ctx, tx)
	}
	return c, nil
}

// isSeqConflict reports an append that lost a race with another process: a duplicate
// sequence (SQLite, or Postgres 23505 on idx_audit_seq) or SQLite's SQLITE_BUSY when a
// deferred transaction cannot take the write lock. Retrying from a fresh tail fixes both.
func isSeqConflict(err error) bool {
	if err == nil {
		return false
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23505" && pg.ConstraintName == "idx_audit_seq"
	}
	var lite interface{ Code() int }
	if errors.As(err, &lite) && lite.Code()&0xff == 5 { // SQLITE_BUSY and its extended codes
		return true
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: audit_records.seq")
}

func (a *auditStore) LogAudit(ctx context.Context, r *AuditRecord) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditWriteBudget)
	defer cancel()
	err := a.logOnce(ctx, r)
	if isSeqConflict(err) {
		// The failed append dropped the cached chain, so the retry chains onto the new tail.
		err = a.logOnce(ctx, r)
	}
	return err
}

func (a *auditStore) logOnce(ctx context.Context, r *AuditRecord) error {
	tx, err := a.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	finish, err := a.append(ctx, tx, r)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	err = tx.Commit()
	finish(err == nil)
	return err
}

// Ready reports whether the next append can place a chain. It takes no lock: append takes
// the connection (the caller's tx) before mu, so Ready taking mu before waiting on the
// connection would deadlock against a concurrent append on SQLite's single connection.
// place's snapshot is one statement, so Ready needs no transaction of its own.
func (a *auditStore) Ready(ctx context.Context) error {
	_, _, err := a.place(ctx, a.store.db, false)
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
	var unkeyed uint64
	if err := a.store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_records WHERE seq IS NULL").Scan(&unkeyed); err != nil {
		return ChainStatus{}, err
	}
	if unkeyed > 0 {
		return ChainStatus{}, fmt.Errorf("%w: %d audit records carry no digest", ErrAuditUnplaceable, unkeyed)
	}
	// anchor was read in a separate, earlier statement; bounding this scan by anchor.Count is
	// what keeps a concurrent append from being read back as a record "past" the anchor that
	// was current when this call started.
	rows, err := a.store.db.QueryContext(ctx, a.store.rebind(`SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records WHERE seq <= ? ORDER BY seq`), anchor.Count)
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
