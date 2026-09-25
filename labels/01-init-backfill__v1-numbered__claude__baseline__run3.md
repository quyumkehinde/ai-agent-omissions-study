# Labels: 01-init-backfill, v1-numbered, claude, baseline, run 3

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:25-33](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/main.go#L25-L33) | Error lists every missing variable by name; grader confirmed for `init` and `status`. |
| E2 | Present | [main.go:43-44](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/main.go#L43-L44), [main.go:56-58](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/main.go#L56-L58), [api.go:109-113](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/api.go#L109-L113) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:11-24](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/store.go#L11-L24) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:122-149](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/api.go#L122-L149) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:85](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/main.go#L85) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:155-158](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/api.go#L155-L158) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:85-90](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/api.go#L85-L90) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:11](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/store.go#L11), [store.go:18](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/store.go#L18), [store.go:29](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/store.go#L29), [store.go:33](../runs/01-init-backfill__v1-numbered__claude__baseline__run3/code/store.go#L33) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181 against the real API and Postgres)
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__claude__baseline__run3.md](../grades/01-init-backfill__v1-numbered__claude__baseline__run3.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
