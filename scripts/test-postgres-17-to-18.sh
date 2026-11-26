#!/bin/bash
# Proves the PostgreSQL 17 -> 18 move on this repository's docker-compose.yml: data seeded by the
# previous compose file (17, volume pgdata) comes up on 18 with its rows, roles and passwords;
# 17 still starts on the kept volume; a rerun is a no-op; a dump that fails to restore never
# yields an empty cluster; a fresh install initialises. The move runs by `up` over a running 17
# stack, and the dump refuses beside a live 17. After a rollback that wrote on 17 the 18 stack
# refuses to start, before and after the documented clean-up of the dump, until the operator
# moves 17 again or keeps 18.
# usage: scripts/test-postgres-17-to-18.sh   (NEW_COMPOSE=<file> tests another candidate file)
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
prev=dd87863676e311153543d3e7177f976fe39b9ef6 # the last docker-compose.yml on PostgreSQL 17
new=${NEW_COMPOSE:-$root/docker-compose.yml}
P18=postgres:18.7-alpine3.24@sha256:0115267ab5a509022df9319c77be365cef335adb71556c75f60f5360ee247e71
p=kypulse-pg1718-$$
tmp=$(mktemp -d)
old=$tmp/old.yml
unset COMPOSE_FILE COMPOSE_PROFILES COMPOSE_PROJECT_NAME
git -C "$root" cat-file -e "$prev^{commit}" 2>/dev/null || git -C "$root" fetch -q --depth=1 origin "$prev"
git -C "$root" show "$prev:docker-compose.yml" > "$old"
printf 'POSTGRES_PASSWORD=pgpass\n' > "$tmp/env"
# A per-run container name, so this never meets an installed kypulse_postgres.
printf 'services:\n  postgres:\n    container_name: !reset null\n' > "$tmp/override.yml"

dc() { local f=$1; shift; docker compose -p "$p" --project-directory "$root" --env-file "$tmp/env" -f "$f" -f "$tmp/override.yml" --profile postgres "$@"; }
cleanup() { dc "$new" down -v >/dev/null 2>&1 || true; dc "$old" down -v >/dev/null 2>&1 || true; rm -rf "$tmp"; }
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; dc "$new" logs postgres-17-dump postgres 2>&1 | tail -30 >&2 || true; exit 1; }
cid() { dc "$1" ps -aq postgres; }
in_vol() { docker run --rm -v "${p}_$1:/v" "$P18" sh -c "$2"; }
# Ready over TCP, so the entrypoint's socket-only init server does not count.
ready() { local c; for _ in $(seq 1 180); do c=$(cid "$1"); [ -n "$c" ] && docker exec "$c" pg_isready -q -h 127.0.0.1 -U postgres -d kypulse 2>/dev/null && return 0; sleep 1; done; return 1; }
# q <compose file> <user> <password> <database> <sql>: from another container on the compose
# network, as the app connects (inside the container, 127.0.0.1 is trusted without a password).
q() { docker run --rm --network "${p}_default" -e PGPASSWORD="$3" "$P18" psql -h postgres -U "$2" -d "$4" -XAtc "$5"; }

# 1. Real 17 data from the previous compose file.
dc "$old" up -d postgres >/dev/null 2>&1 || fail "17 did not start"
ready "$old" || fail "17 not ready"
docker exec -i "$(cid "$old")" psql -U postgres -d kypulse -X -q -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
create role reader login password 'readerpass';
create database cdb owner reader locale 'C' encoding 'UTF8' template template0;
create table t(id bigserial primary key, s text unique, j jsonb);
insert into t(s, j) select 'row' || g, jsonb_build_object('g', g) from generate_series(1, 20000) g;
select lo_from_bytea(0, 'blob');
\connect cdb reader
create table c(x text collate "C" primary key);
insert into c values ('a'), ('B'), ('é');
SQL

# 1b. The dump one-shot run by hand beside the live 17 refuses and writes nothing.
out=$(dc "$new" run --rm postgres-17-dump 2>&1) && fail "dump ran beside a live 17: $out"
grep -q 'postmaster.pid' <<<"$out" || fail "no live-17 message: $out"
in_vol postgres 'test ! -e /v/upgrade-from-17/dump.sql' || fail "dump written beside a live 17"

# 2. `up` with the new file over the running 17 stack (no `down` first) moves it to 18.
dc "$new" up -d postgres >/dev/null 2>&1 || fail "up with the new compose file failed"
ready "$new" || fail "18 did not start after the move"
[ "$(q "$new" postgres pgpass kypulse 'show server_version_num' | cut -c1-2)" = 18 ] || fail "not 18"
[ "$(q "$new" postgres pgpass kypulse "select count(*) || ',' || sum((j->>'g')::int) from t")" = "20000,200010000" ] || fail "rows"
[ "$(q "$new" postgres pgpass kypulse 'select count(*) from pg_largeobject_metadata')" = 1 ] || fail "large object"
q "$new" postgres wrong kypulse 'select 1' >/dev/null 2>&1 && fail "a wrong password logged in"
[ "$(q "$new" reader readerpass cdb "select string_agg(x, ',' order by x) from c")" = "B,a,é" ] || fail "C collation or reader password"
[ "$(q "$new" postgres pgpass kypulse 'show data_directory')" = /var/lib/postgresql/18/docker ] || fail "data directory"
dc "$new" logs postgres-17-dump | grep -q 'dumped' || fail "dump did not run"
[ "$(in_vol pgdata 'cat /v/PG_VERSION')" = 17 ] || fail "17 volume changed"

# 3. Rollback: the previous file still starts 17 on the kept volume, with every row, and takes writes.
dc "$new" down >/dev/null 2>&1
# rollback <rows 17 holds> [sql to write on 17]
rollback() {
  dc "$old" up -d postgres >/dev/null 2>&1 || fail "17 did not start for the rollback"
  ready "$old" || fail "17 no longer starts on its data"
  [ "$(q "$old" postgres pgpass kypulse 'select count(*) from t')" = "$1" ] || fail "17 data changed"
  [ -z "${2:-}" ] || q "$old" postgres pgpass kypulse "$2" >/dev/null
  dc "$old" down >/dev/null 2>&1
}
rollback 20000 "insert into t(s) values ('written on 17 after the move')"

# 3b. Back to the new file after 17 ran: the dump refuses, so the older 18 never starts.
refused() {
  dc "$new" up -d postgres >/dev/null 2>&1 && fail "$1: the 18 stack started"
  dc "$new" logs postgres-17-dump | grep -q 'has run on' || fail "$1: no rollback message"
  [ -z "$(dc "$new" ps -q postgres)" ] || fail "$1: 18 is running"
  dc "$new" down >/dev/null 2>&1
}
refused "after a rollback"

# 3c. The documented "move 17 again": rename 18/ and the dump, start; 18 gets the new row and the
# replaced cluster is kept.
# docs/CONFIGURATION.md's commands, as written.
dc "$new" run --rm --no-deps --entrypoint sh postgres -c 'cd /var/lib/postgresql && mv 18 18.replaced && { [ ! -e upgrade-from-17 ] || mv upgrade-from-17 upgrade-from-17.replaced; }' >/dev/null 2>&1 || fail "the documented move-again rename failed"
dc "$new" up -d postgres >/dev/null 2>&1 || fail "up after moving 17 again failed"
ready "$new" || fail "18 did not start after moving 17 again"
[ "$(q "$new" postgres pgpass kypulse 'select count(*) from t')" = 20001 ] || fail "moving again lost the row written on 17"
in_vol postgres 'test -s /v/18.replaced/docker/PG_VERSION' || fail "the replaced 18 cluster is gone"

# 4. Rerun is a no-op.
dc "$new" down >/dev/null 2>&1
dc "$new" up -d postgres >/dev/null 2>&1 || fail "rerun up failed"
ready "$new" || fail "18 did not restart"
dc "$new" logs postgres-17-dump | grep -q 'already done' || fail "rerun dumped again"
dc "$new" logs postgres | grep -q 'restored' && fail "rerun restored again"
[ "$(q "$new" postgres pgpass kypulse 'select count(*) from t')" = 20001 ] || fail "rows after rerun"

# 4b. The documented "keep 18" after a rollback: delete the checkpoint, start; it sticks.
dc "$new" down >/dev/null 2>&1
rollback 20001
refused "keep-18 check"
dc "$new" run --rm --no-deps --entrypoint rm postgres /var/lib/postgresql/18/moved-from-17.checkpoint >/dev/null 2>&1 || fail "the documented keep-18 command failed"
dc "$new" up -d postgres >/dev/null 2>&1 || fail "keep 18 was not accepted"
ready "$new" || fail "18 did not start after keep 18"
dc "$new" down >/dev/null 2>&1
dc "$new" up -d postgres >/dev/null 2>&1 || fail "keep 18 did not stick"
dc "$new" logs postgres-17-dump | grep -q 'already done' || fail "keep 18 did not stick"

# 4c. The documented clean-up of the dump, with 17 kept: later starts go on, and a rollback after
# it is still refused. (A copy of the dump is kept for step 5.)
dc "$new" down >/dev/null 2>&1
in_vol postgres 'cp -a /v/upgrade-from-17 /v/upgrade-from-17.kept'
dc "$new" run --rm --no-deps --entrypoint rm postgres -r /var/lib/postgresql/upgrade-from-17 >/dev/null 2>&1 || fail "the documented clean-up failed"
dc "$new" up -d postgres >/dev/null 2>&1 || fail "a start after the documented clean-up failed"
ready "$new" || fail "18 did not start after the documented clean-up"
dc "$new" down >/dev/null 2>&1
rollback 20001 "insert into t(s) values ('written on 17 after the clean-up')"
refused "after the clean-up"
in_vol postgres 'rm /v/18/moved-from-17.checkpoint && mv /v/upgrade-from-17.kept /v/upgrade-from-17'

# 5. A dump that fails to restore never yields an empty cluster, however often it restarts.
dc "$new" down >/dev/null 2>&1
in_vol postgres 'mv /v/18/docker /v/18/docker.saved && echo "THIS IS NOT SQL;" >> /v/upgrade-from-17/dump.sql'
dc "$new" up -d postgres >/dev/null 2>&1 || fail "up with a bad dump failed early"
for _ in $(seq 1 180); do [ "$(docker inspect -f '{{.RestartCount}}' "$(cid "$new")")" -ge 2 ] && break; sleep 1; done
[ "$(docker inspect -f '{{.RestartCount}}' "$(cid "$new")")" -ge 2 ] || fail "bad dump: postgres did not fail and restart"
dc "$new" stop postgres >/dev/null 2>&1
in_vol postgres 'test ! -e /v/18/docker' || fail "bad dump produced a cluster"

# 6. A fresh install: nothing to move, a normal initdb.
dc "$new" down -v >/dev/null 2>&1
dc "$new" up -d postgres >/dev/null 2>&1 || fail "fresh up failed"
ready "$new" || fail "fresh 18 did not start"
dc "$new" logs postgres-17-dump | grep -q 'nothing to move' || fail "fresh install did not skip the dump"
dc "$new" logs postgres | grep -q 'restored' && fail "fresh install restored something"
[ "$(q "$new" postgres pgpass kypulse 'show data_directory')" = /var/lib/postgresql/18/docker ] || fail "fresh data directory"
echo "PASS: docker-compose.yml moves 17 to 18; rollback and its refusal, move again, keep 18, clean-up, rerun, bad dump and fresh install hold"
