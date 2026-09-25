# Labels: 01-init-backfill, v3-terse, claude, baseline, run 4

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:22-40](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/main.go#L22-L40) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:67](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/main.go#L67), [api.go:107-108](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/api.go#L107-L108) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:11-24](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L11-L24) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:118-149](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/api.go#L118-L149), [store.go:60](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L60) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:89](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/main.go#L89) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:155-157](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/api.go#L155-L157) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:87-91](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/api.go#L87-L91) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:11](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L11), [store.go:18](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L18), [store.go:40](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L40), [store.go:52](../runs/01-init-backfill__v3-terse__claude__baseline__run4/code/store.go#L52) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports runs against the live API and the Postgres container)
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__claude__baseline__run4.md](../grades/01-init-backfill__v3-terse__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
