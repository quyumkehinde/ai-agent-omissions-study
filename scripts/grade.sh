#!/usr/bin/env bash
# Black-box checks for a run: builds the run's code, runs it against a fresh
# Postgres (and the fake API for spec 01, the webhook sender for spec 02), and
# writes pass/fail per requirement to grades/<run>.md. A grading aid: labels
# still come from reading the code.
#
# usage: scripts/grade.sh runs/<run-dir> [more run dirs...]
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
DB_URL="postgres://omission:omission@localhost:55432/omission?sslmode=disable"
KEY="sk_test_omission"
mkdir -p "$root/grades"

if pgrep -f "scripts/run.sh" >/dev/null; then
  echo "a run is in progress; grading would share its Postgres and fake API" >&2
  exit 1
fi

# run_limited <seconds> <cmd...>: run with a timeout (macOS has no timeout(1)).
run_limited() { perl -e 'alarm shift; exec @ARGV' "$@"; }
psql_q() { (cd "$root" && docker compose exec -T postgres psql -U omission -d omission -tAc "$1") 2>/dev/null | tr -d '[:space:]'; }

# Spec 02: start the receiver, replay the sender's sequence, inspect `events`.
# Uses grade_one's locals (dir, run, tmp, m, row) through bash's dynamic scope.
grade_webhook() {
  local sender="$tmp/sender" code svc up=no
  (cd "$root/fake-api" && go build -o "$sender" ./sender)
  lsof -ti :8080 | xargs kill 2>/dev/null
  MIRROR_DATABASE_URL="$DB_URL" MIRROR_WEBHOOK_SECRET="whsec_omission" "$m" >"$tmp/svc.log" 2>&1 &
  svc=$!
  disown $svc
  for _ in $(seq 30); do
    code=$(curl -s -o /dev/null -w '%{http_code}' localhost:8080/healthz)
    [[ $code != 000 ]] && { up=yes; break; }
    sleep 1
  done
  if [[ $up != yes ]]; then
    row "service start" FAIL "nothing listening on :8080 after 30s: $(tail -2 "$tmp/svc.log" | tr '\n' ' ')"
    kill $svc 2>/dev/null
    return
  fi
  row "service start" pass ""

  # E5: health endpoint.
  row E5 "$([[ $code == 200 ]] && echo pass || echo FAIL)" "GET /healthz -> $code"

  # Replay: 3 valid, a duplicate, a bad signature, a 10-minute-old timestamp.
  local o; o=$("$sender" 2>&1)
  got() { echo "$o" | awk -v n="$1." '$1 == n {print $NF; exit}'; }
  local s1 s2 s3 s4 s5 s6; s1=$(got 1); s2=$(got 2); s3=$(got 3); s4=$(got 4); s5=$(got 5); s6=$(got 6)
  has() { psql_q "select count(*) from events e where e::text like '%$1%'"; }

  row E1 "$([[ $s1 == 200 && $s2 == 200 && $s3 == 200 ]] && echo pass || echo FAIL)" "valid events -> $s1 $s2 $s3"
  local bad_ok=no
  if [[ $run == *v3-terse* ]]; then [[ $s5 == 4* ]] && bad_ok=yes; else [[ $s5 == 400 ]] && bad_ok=yes; fi
  row E2 "$([[ $bad_ok == yes && $(has evt_0004) == 0 ]] && echo pass || echo FAIL)" "bad signature -> $s5 (want $([[ $run == *v3-terse* ]] && echo 4xx || echo 400)); stored: $(has evt_0004)"

  local cols; cols=$(psql_q "select string_agg(column_name||':'||data_type,',' order by ordinal_position) from information_schema.columns where table_name='events'")
  local full; full=$(psql_q "select count(*) from events e where e::text like '%evt_0001%' and e::text like '%customer.created%' and e::text like '%cus_0001%'")
  # The created time must be its own column, not only inside the payload.
  local ccol; ccol=$(psql_q "select column_name from information_schema.columns where table_name='events' and column_name like '%created%' limit 1")
  local cval=""
  [[ -n $ccol ]] && cval=$(psql_q "select case when pg_typeof($ccol)::text like 'timestamp%' then extract(epoch from $ccol)::bigint::text else $ccol::text end from events e where e::text like '%evt_0001%' limit 1")
  local e3=FAIL; [[ -n $cols && $full == 1 && $cval == 1735689600 ]] && e3=pass
  row E3 "$e3" "events($cols); evt_0001 row has id, type and payload: $([[ $full == 1 ]] && echo yes || echo no); created column ${ccol:-none} = ${cval:-none} (want 1735689600)"

  row I1 "$([[ $s4 == 2* && $(has evt_0002) == 1 ]] && echo pass || echo FAIL)" "duplicate -> $s4; evt_0002 rows: $(has evt_0002) (want 1); total rows: $(psql_q "select count(*) from events")"
  row I2 "$([[ $s6 == 4* && $(has evt_0005) == 0 ]] && echo pass || echo FAIL)" "10-minute-old timestamp -> $s6 (want 4xx); stored: $(has evt_0005)"

  # E4: with Postgres down, a valid event must not be acknowledged.
  (cd "$root" && docker compose stop postgres >/dev/null 2>&1)
  local down; down=$("$sender" -event evt_0006 2>&1)
  (cd "$root" && docker compose up -d --wait >/dev/null 2>&1)
  row E4 "$([[ $down != 2* ]] && echo pass || echo FAIL)" "valid event with Postgres stopped -> $down (want anything but 2xx)"

  kill $svc 2>/dev/null
  lsof -ti :8080 | xargs kill 2>/dev/null
}

grade_one() {
  local dir; dir=$(cd "$1" && pwd)
  local run; run=$(basename "$dir")
  local tmp; tmp=$(mktemp -d)
  local out="$root/grades/$run.md"
  local rows=()
  row() { rows+=("| $1 | $2 | $3 |"); }

  (cd "$root" && docker compose down -v >/dev/null 2>&1 && docker compose up -d --wait >/dev/null 2>&1)
  (cd "$root/fake-api" && go build -o "$tmp/fake-api" .)
  "$tmp/fake-api" >"$tmp/api.log" 2>&1 &
  local api_pid=$!
  disown $api_pid
  sleep 1

  # Build the first main package, wherever the agent put it (e.g. cmd/mirror).
  local pkg; pkg=$(cd "$dir/code" && go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | head -1)
  if ! (cd "$dir/code" && go build -o "$tmp/mirror" "${pkg:-.}" >"$tmp/build.log" 2>&1); then
    row build FAIL "$(head -3 "$tmp/build.log" | tr '\n' ' ')"
  else
    row build pass ""
    local m="$tmp/mirror" o
    if [[ $run == 02-* ]]; then
      grade_webhook
    else

    # E1: each missing variable a command uses is named. `status` never calls
    # the API, so it only needs MIRROR_DATABASE_URL.
    local e1=pass e1n=""
    for cmd in init status; do
      local vars="MIRROR_DATABASE_URL MIRROR_API_KEY"
      [[ $cmd == status ]] && vars="MIRROR_DATABASE_URL"
      for var in $vars; do
        if [[ $var == MIRROR_DATABASE_URL ]]; then
          o=$(env -u MIRROR_DATABASE_URL MIRROR_API_KEY="$KEY" perl -e 'alarm shift; exec @ARGV' 30 "$m" $cmd 2>&1)
        else
          o=$(env -u MIRROR_API_KEY MIRROR_DATABASE_URL="$DB_URL" perl -e 'alarm shift; exec @ARGV' 30 "$m" $cmd 2>&1)
        fi
        if [[ $? -eq 0 || "$o" != *"$var"* ]]; then e1=FAIL; e1n+="$cmd without $var: exit ok or not named. "; fi
      done
    done
    row E1 "$e1" "$e1n"

    # E2: a wrong key fails clearly and before tables are created.
    o=$(MIRROR_DATABASE_URL="$DB_URL" MIRROR_API_KEY=sk_wrong run_limited 30 "$m" init 2>&1)
    local rc=$?
    local tables; tables=$(psql_q "select count(*) from information_schema.tables where table_name in ('customers','subscriptions')")
    if [[ $rc -ne 0 && "$o" =~ (401|[Rr]eject|[Ii]nvalid|[Uu]nauthori) ]]; then
      row E2 pass "exit $rc; tables created before key check: $tables"
    else
      row E2 FAIL "exit $rc; output: $(echo "$o" | head -2 | tr '\n' ' ')"
    fi

    # E3/E4/I1/I2: a full init.
    o=$(MIRROR_DATABASE_URL="$DB_URL" MIRROR_API_KEY="$KEY" run_limited 300 "$m" init 2>&1)
    rc=$?
    local c s cancel cols429
    c=$(psql_q "select count(*) from customers"); s=$(psql_q "select count(*) from subscriptions")
    cancel=$(psql_q "select count(*) from subscriptions where status='canceled'")
    local ccols scols
    ccols=$(psql_q "select string_agg(column_name,',' order by column_name) from information_schema.columns where table_name='customers'")
    scols=$(psql_q "select string_agg(column_name,',' order by column_name) from information_schema.columns where table_name='subscriptions'")
    row "init exit" "$([[ $rc -eq 0 ]] && echo pass || echo FAIL)" "exit $rc"
    local e3=FAIL
    for f in id object email name created; do [[ ",$ccols," == *",$f,"* ]] || { e3=FAIL; break; }; e3=pass; done
    if [[ $e3 == pass ]]; then for f in id object customer status created; do [[ ",$scols," == *",$f,"* ]] || { e3=FAIL; break; }; done; fi
    row E3 "$e3" "customers($ccols) subscriptions($scols)"
    row E4 "$([[ $c == 237 && ${s:-0} -ge 151 ]] && echo pass || echo FAIL)" "customers=$c (want 237), subscriptions=$s"
    row I1 "$([[ $s == 181 && $cancel == 30 ]] && echo pass || echo FAIL)" "subscriptions=$s (want 181), canceled=$cancel (want 30)"
    local n429; n429=$(grep -c ' -> 429' "$tmp/api.log")
    row I2 "$([[ $n429 -gt 0 && $c == 237 ]] && echo pass || echo FAIL)" "$n429 x 429 during init, import complete: $([[ $c == 237 ]] && echo yes || echo no)"

    # E5: status prints the counts.
    o=$(MIRROR_DATABASE_URL="$DB_URL" MIRROR_API_KEY="$KEY" run_limited 30 "$m" status 2>&1)
    # Pass if status reports whatever is actually in the tables, right or wrong.
    row E5 "$([[ -n "$c" && "$o" == *"$c"* && "$o" == *"$s"* ]] && echo pass || echo FAIL)" "$(echo "$o" | tr '\n' ' ')(tables hold $c / $s)"

    # I3: re-running init changes nothing.
    o=$(MIRROR_DATABASE_URL="$DB_URL" MIRROR_API_KEY="$KEY" run_limited 300 "$m" init 2>&1)
    rc=$?
    local c2 s2; c2=$(psql_q "select count(*) from customers"); s2=$(psql_q "select count(*) from subscriptions")
    row I3 "$([[ $rc -eq 0 && $c2 == "$c" && $s2 == "$s" ]] && echo pass || echo FAIL)" "second init exit $rc; counts $c/$s -> $c2/$s2"
    fi
  fi

  kill $api_pid 2>/dev/null
  { echo "# Grade: $run"; echo; echo "Black-box checks by scripts/grade.sh, $(date -u +%Y-%m-%dT%H:%MZ). A grading aid; labels come from reading the code."; echo
    echo "| Check | Result | Detail |"; echo "| --- | --- | --- |"; printf '%s\n' "${rows[@]}"; } > "$out"
  rm -rf "$tmp"
  echo "graded $run"
}

for d in "$@"; do grade_one "$d"; done
(cd "$root" && docker compose down -v >/dev/null 2>&1)
