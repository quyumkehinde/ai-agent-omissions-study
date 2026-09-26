# Labels: 01-init-backfill, v2-reordered, codex, baseline, run 1

Run date: 2026-09-25. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:27-34](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/main.go#L27-L34) | Names the missing variable, one at a time, for both commands; grader confirmed. |
| E2 | Present | [main.go:36-41](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/main.go#L36-L41), [api.go:48-54](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L48-L54) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [database.go:14-29](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/database.go#L14-L29) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:81-100](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L81-L100), [api.go:103-153](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L103-L153) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [database.go:30](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/database.go#L30) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:129-132](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L129-L132) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:161-165](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L161-L165) | Retries a 429 after `Retry-After`, up to 3 attempts; grader: import completed through a 429. |
| I3 | Present | [database.go:15](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/database.go#L15), [database.go:18](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/database.go#L18), [api.go:84](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L84), [api.go:95](../runs/01-init-backfill__v2-reordered__codex__baseline__run1/code/api.go#L95) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes, based on `go test`, `go vet` and `go build` only. It never ran the tool: the fake API log has no requests during the run. The message doesn't claim a live run.
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__codex__baseline__run1.md](../grades/01-init-backfill__v2-reordered__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
