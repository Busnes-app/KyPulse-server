#!/usr/bin/env bash
# Refuse classical TLS overrides and prove outbound PQ hybrid TLS (X25519MLKEM768)
# under both default and GODEBUG=fips140=on profiles.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

check_absent() {
  local label="$1"
  local pattern="$2"
  shift 2
  local matches=""
  local rc=0
  matches=$(git grep -n -E -- "$pattern" "$@") || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "$label:" >&2
    echo "$matches" >&2
    exit 1
  elif [ "$rc" -ne 1 ]; then
    exit "$rc"
  fi
}

check_absent \
  "CurvePreferences must not be set in non-test Go code (use Go's default hybrid PQ curves)" \
  'CurvePreferences' \
  -- '*.go' ':!*_test.go'

check_absent \
  "tlsmlkem override must not appear in non-test code, workflows, Dockerfiles, or Compose files" \
  'tlsmlkem' \
  -- . ':!*_test.go' ':!*.md' ':!scripts/check-pqc-tls.sh'

run_pq_test() {
  local mode="$1"
  shift
  local out
  if ! out=$("$@" go test -v -count=1 -run '^TestOutboundPQTLS$' ./internal/egress 2>&1); then
    echo "PQ TLS test failed ($mode):" >&2
    echo "$out" >&2
    exit 1
  fi
  if ! grep -q '^--- PASS: TestOutboundPQTLS ' <<< "$out"; then
    echo "Expected TestOutboundPQTLS to run and pass ($mode):" >&2
    echo "$out" >&2
    exit 1
  fi
}

run_pq_test "default" env -u GODEBUG
run_pq_test "fips140=on" env GODEBUG=fips140=on
