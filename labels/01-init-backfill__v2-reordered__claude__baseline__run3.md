# Labels: 01-init-backfill, v2-reordered, claude, baseline, run 3

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:26-37](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/main.go#L26-L37), [main.go:47-62](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/main.go#L47-L62) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:68](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/main.go#L68), [api.go:111-112](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/api.go#L111-L112) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [db.go:15-28](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L15-L28) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:126-160](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/api.go#L126-L160), [db.go:56](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L56) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:93](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/main.go#L93) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:167-168](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/api.go#L167-L168) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:85-92](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/api.go#L85-L92) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [db.go:15](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L15), [db.go:22](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L22), [db.go:39](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L39), [db.go:49](../runs/01-init-backfill__v2-reordered__claude__baseline__run3/code/db.go#L49) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181)
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__claude__baseline__run3.md](../grades/01-init-backfill__v2-reordered__claude__baseline__run3.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
