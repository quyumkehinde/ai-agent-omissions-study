# Labels: 01-init-backfill, v3-terse, claude, baseline, run 3

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [cmd.go:19-40](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/cmd.go#L19-L40) | Error names each missing variable, for both commands; grader confirmed. |
| E2 | Present | [cmd.go:44](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/cmd.go#L44), [api.go:128-132](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/api.go#L128-L132) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [db.go:11-24](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/db.go#L11-L24) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:141-168](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/api.go#L141-L168), [cmd.go:62](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/cmd.go#L62) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [cmd.go:83](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/cmd.go#L83) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:174-177](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/api.go#L174-L177) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:92-99](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/api.go#L92-L99) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [db.go:11](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/db.go#L11), [db.go:18](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/db.go#L18), [db.go:42](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/db.go#L42), [db.go:55](../runs/01-init-backfill__v3-terse__claude__baseline__run3/code/db.go#L55) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181, run against a scratch database it created in the provided Postgres)
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__claude__baseline__run3.md](../grades/01-init-backfill__v3-terse__claude__baseline__run3.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
