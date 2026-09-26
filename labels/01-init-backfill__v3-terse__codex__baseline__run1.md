# Labels: 01-init-backfill, v3-terse, codex, baseline, run 1

Run date: 2026-09-25. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:25-36](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/main.go#L25-L36) | `init` names the missing variable, one at a time; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:47-49](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/main.go#L47-L49), [api.go:45-51](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/api.go#L45-L51) | Rejected key fails before tables are created (the database handle is opened first); grader: wrong key exits 1, no tables created. |
| E3 | Present | [database.go:20-35](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L20-L35) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:77-86](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/api.go#L77-L86), [api.go:88-135](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/api.go#L88-L135) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [database.go:42](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L42) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:81](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/api.go#L81) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:98-103](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/api.go#L98-L103) | Waits for `Retry-After` and retries; grader: import completed through a 429. |
| I3 | Present | [database.go:22](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L22), [database.go:29](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L29), [database.go:65](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L65), [database.go:73](../runs/01-init-backfill__v3-terse__codex__baseline__run1/code/database.go#L73) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes, based on `go test`, `go vet` and `go build` only. It never ran the tool: the fake API log has no requests during the run. The message doesn't claim a live run.
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__codex__baseline__run1.md](../grades/01-init-backfill__v3-terse__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
