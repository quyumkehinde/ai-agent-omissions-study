# Labels: 01-init-backfill, v1-numbered, claude, baseline, run 5

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:27-36](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/main.go#L27-L36) | Error lists every missing variable by name; grader confirmed for `init` and `status`. |
| E2 | Present | [main.go:50](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/main.go#L50), [main.go:58-60](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/main.go#L58-L60), [api.go:126-130](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/api.go#L126-L130) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:12-25](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/store.go#L12-L25) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:142-169](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/api.go#L142-L169) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:89](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/main.go#L89) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:175-177](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/api.go#L175-L177) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:90-98](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/api.go#L90-L98) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:12](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/store.go#L12), [store.go:19](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/store.go#L19), [store.go:53](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/store.go#L53), [store.go:71](../runs/01-init-backfill__v1-numbered__claude__baseline__run5/code/store.go#L71) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181 against the real API and Postgres)
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__claude__baseline__run5.md](../grades/01-init-backfill__v1-numbered__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
