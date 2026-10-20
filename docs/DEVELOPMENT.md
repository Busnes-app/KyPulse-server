# Development

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## API

Routes answer JSON, errors are `{"error": "..."}`, and state-changing browser requests need
the `ky_csrf` cookie echoed in `X-CSRF-Token`. [`internal/api/AGENTS.md`](../internal/api/AGENTS.md)
has every request and response shape.

| Route | Who |
|---|---|
| `GET /healthz` | public, `ky.health/1` |
| `GET /api/status`, `/api/targets`, `/api/targets/{id}`, `/api/alerts`, `/api/kyyard` | any signed-in user |
| `POST/PUT/DELETE /api/targets…`, `POST /api/targets/{id}/silence` | admin |
| `GET/PUT/DELETE /api/alerts/webhook`, `POST /api/alerts/webhook/test` | admin |
| `POST /api/kyyard/pair`, `DELETE /api/kyyard`, `GET /api/kyyard/containers` | admin |
| `POST /api/log-sources/pairing`, `GET /api/log-sources`, `DELETE /api/log-sources/{id}` | admin |
| `POST /api/log-sources/claim` | public |
| `POST /api/ingest/logs` | source Bearer token |
| `GET /api/logs`, `GET /api/activity` | admin |
| `/api/backup/…` | admin |

## Develop

Go 1.26 and Node 22.

```sh
make ci             # tidy check, gofmt, vet, race tests, frontend tests, smoke test
make run            # build and run on :8080 with ./data
make test-postgres  # the Go suite against KYPULSE_TEST_POSTGRES_DSN
```

The frontend lives in `web/` and is embedded from `web/dist`, which is committed; rebuild it
with `cd web && npm ci && npm run build` after any frontend change, because CI diffs it.
Browser regressions run with `go build -o .browser/server ./cmd/server`, then
`cd web && npx playwright install chromium && npm run test:browser`; they need a private IPv4
interface for the fake watched app. Contributor rules live in [`AGENTS.md`](../AGENTS.md) and the
`AGENTS.md` beside each package.
