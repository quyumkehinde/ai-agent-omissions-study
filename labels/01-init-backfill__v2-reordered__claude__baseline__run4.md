# Labels: 01-init-backfill, v2-reordered, claude, baseline, run 4

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:40-53](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/main.go#L40-L53) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:63-68](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/main.go#L63-L68), [main.go:79](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/main.go#L79), [api.go:112-113](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/api.go#L112-L113) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:10-23](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/store.go#L10-L23) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:118-152](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/api.go#L118-L152) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:110](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/main.go#L110) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:160-161](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/api.go#L160-L161) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:87-94](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/api.go#L87-L94) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:10](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/store.go#L10), [store.go:17](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/store.go#L17), [store.go:38](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/store.go#L38), [store.go:53](../runs/01-init-backfill__v2-reordered__claude__baseline__run4/code/store.go#L53) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181, including 30 canceled)
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__claude__baseline__run4.md](../grades/01-init-backfill__v2-reordered__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
