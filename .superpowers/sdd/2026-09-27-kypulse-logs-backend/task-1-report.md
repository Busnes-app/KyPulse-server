# Task 1 report: atomic log and activity storage

Status: complete. Implementation commit: `0061287` (`logs: add atomic storage and bounded parsing`).

Changes:
- Migration 7 creates `log_lines`, `activity`, and the single-row `log_usage` counter for SQLite and PostgreSQL. Target foreign keys use `ON DELETE SET NULL`; external activity keys have a partial unique index.
- `Store.Logs()` exposes atomic append, filtered ID pages, and seven-day/byte-cap pruning. Append and prune lock `log_usage` first. Source IDs are rejected until task 2 adds active-source persistence; empty IDs serve internal KyYard collection. Imported activity stays outside the keyed audit trail.
- `logstore.Parse` bounds raw lines to 16 KiB at a UTF-8 boundary, extracts suite logging fields and audit-shaped activity, and falls back to transport/receive time. `Display` strips terminal escapes and controls while preserving literal angle brackets.
- Tests cover real ky-primitives Logger/Audit output, parsing fallbacks and bounds, transactional rollback after activity failure, usage accounting, replay/collision handling, filters, pages, target deletion and age pruning. Package DOX is updated.

Validation:
- `go test ./internal/store ./internal/logstore` — pass (SQLite).
- `go test -race ./internal/store ./internal/logstore` — pass (SQLite).
- `KYPULSE_TEST_POSTGRES_DSN='postgres://ci:ci@127.0.0.1:15441/ci_test?sslmode=disable' go test ./internal/store ./internal/logstore` — pass.
- `KYPULSE_TEST_POSTGRES_DSN='postgres://ci:ci@127.0.0.1:15441/ci_test?sslmode=disable' go test -race ./internal/store ./internal/logstore` — pass.
- `go test ./...` — pass after initial implementation; scoped tests rerun after the replay fix.
- `git diff --check` — pass.

Concerns and follow-up:
- Task 2 must replace the deliberate non-empty source ID rejection with a same-transaction active-source check before ingest is exposed.
- Task 4 owns the configured cap and hourly retention loop. Current `Append` enforces its supplied byte cap; `Prune` performs seven-day cleanup when called with a nonzero clock.
- The requested red-test stage was not recorded before implementation. All final tests above passed.
