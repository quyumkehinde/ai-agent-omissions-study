# Labels: 01-init-backfill, v2-reordered, claude, baseline, run 1

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:24-34](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/main.go#L24-L34), [main.go:42](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/main.go#L42) | Error names each missing variable; grader confirmed for `init` and `status`. |
| E2 | Present | [main.go:56-63](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/main.go#L56-L63), [api.go:111-115](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/api.go#L111-L115) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:14-27](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/store.go#L14-L27) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:127-154](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/api.go#L127-L154) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:93](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/main.go#L93) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:161-162](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/api.go#L161-L162) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:79-86](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/api.go#L79-L86) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:14](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/store.go#L14), [store.go:21](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/store.go#L21), [store.go:55](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/store.go#L55), [store.go:69](../runs/01-init-backfill__v2-reordered__claude__baseline__run1/code/store.go#L69) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181 against the real API and Postgres)
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__claude__baseline__run1.md](../grades/01-init-backfill__v2-reordered__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
