# Labels: 01-init-backfill, v1-numbered, claude, baseline, run 4

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:25-35](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/main.go#L25-L35), [main.go:44](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/main.go#L44) | Error lists every missing variable by name; grader confirmed for `init` and `status`. |
| E2 | Present | [main.go:51](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/main.go#L51), [main.go:81](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/main.go#L81), [api.go:116-118](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/api.go#L116-L118) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:12-25](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/store.go#L12-L25) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:130-157](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/api.go#L130-L157) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:102](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/main.go#L102) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:163-165](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/api.go#L163-L165) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:85-93](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/api.go#L85-L93) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:12](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/store.go#L12), [store.go:19](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/store.go#L19), [store.go:54](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/store.go#L54), [store.go:65](../runs/01-init-backfill__v1-numbered__claude__baseline__run4/code/store.go#L65) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes (reports 237 / 181 and a clean second `init`)
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__claude__baseline__run4.md](../grades/01-init-backfill__v1-numbered__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
