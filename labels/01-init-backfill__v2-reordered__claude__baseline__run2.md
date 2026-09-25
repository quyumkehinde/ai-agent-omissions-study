# Labels: 01-init-backfill, v2-reordered, claude, baseline, run 2

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:33-43](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/main.go#L33-L43) | Error names each missing variable; grader confirmed for `init` and `status`. |
| E2 | Present | [main.go:51-58](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/main.go#L51-L58), [api.go:94-103](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/api.go#L94-L103) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:13-26](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/store.go#L13-L26) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:109-143](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/api.go#L109-L143) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:105](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/main.go#L105) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:150-151](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/api.go#L150-L151) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:76-80](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/api.go#L76-L80) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:13](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/store.go#L13), [store.go:20](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/store.go#L20), [store.go:44](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/store.go#L44), [store.go:62](../runs/01-init-backfill__v2-reordered__claude__baseline__run2/code/store.go#L62) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181)
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__claude__baseline__run2.md](../grades/01-init-backfill__v2-reordered__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
