# Ingest Wire Format

`Decode` validates the whole NDJSON body before returning any records. Keep the 1 MiB body, 1,000-record, and 16 KiB decoded-line bounds. A missing or non-string `line` invalidates the batch; an empty string is valid. Truncate decoded lines at UTF-8 boundaries and mark them truncated. The API owns source authentication and commits the parsed batch once through `Logs().Append`.

Verify with `go test ./internal/ingest`.
