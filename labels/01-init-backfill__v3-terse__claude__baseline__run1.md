# Labels: 01-init-backfill, v3-terse, claude, baseline, run 1

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:38-50](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L38-L50), [main.go:71](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L71), [main.go:104](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L104) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:29](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L29), [main.go:79-85](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L79-L85), [api.go:129-133](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/api.go#L129-L133) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [db.go:12-25](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L12-L25) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:142-169](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/api.go#L142-L169), [db.go:79](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L79) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:103](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/main.go#L103) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:175-178](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/api.go#L175-L178) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:86-92](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/api.go#L86-L92) | Waits for `Retry-After`; also retries network errors and 5xx. Grader: import completed through a 429. |
| I3 | Present | [db.go:12](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L12), [db.go:19](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L19), [db.go:44](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L44), [db.go:65](../runs/01-init-backfill__v3-terse__claude__baseline__run1/code/db.go#L65) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181 and an unchanged second `init`)
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__claude__baseline__run1.md](../grades/01-init-backfill__v3-terse__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
