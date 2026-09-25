# Labels: 01-init-backfill, v3-terse, claude, baseline, run 2

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:57-66](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L57-L66), [main.go:74](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L74), [main.go:112](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L112) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:39](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L39), [main.go:82-90](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L82-L90), [api.go:135-137](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/api.go#L135-L137) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [db.go:11-24](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/db.go#L11-L24) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:144-168](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/api.go#L144-L168), [backfill.go:17](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/backfill.go#L17) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:111](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/main.go#L111) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:174-176](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/api.go#L174-L176) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:100-107](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/api.go#L100-L107) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [db.go:11](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/db.go#L11), [db.go:18](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/db.go#L18), [db.go:40](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/db.go#L40), [db.go:53](../runs/01-init-backfill__v3-terse__claude__baseline__run2/code/db.go#L53) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181)
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__claude__baseline__run2.md](../grades/01-init-backfill__v3-terse__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
