# Labels: 01-init-backfill, v1-numbered, codex, baseline, run 1

Run date: 2026-09-25. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:33-40](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L33-L40) | Names the missing variable, one at a time, for both commands; grader confirmed. |
| E2 | Present | [main.go:58-61](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L58-L61), [api.go:38-44](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/api.go#L38-L44) | Rejected key (401 or 403) fails before tables are created, though it connects to Postgres first; grader: wrong key exits 1, no tables created. |
| E3 | Present | [main.go:73-88](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L73-L88) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:53-57](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/api.go#L53-L57), [api.go:60-90](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/api.go#L60-L90), [main.go:89](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L89) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:123](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L123) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:56-57](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/api.go#L56-L57) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:107-118](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/api.go#L107-L118) | Waits for `Retry-After` and retries; grader: import completed through a 429. |
| I3 | Present | [main.go:74](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L74), [main.go:77](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L77), [main.go:105](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L105), [main.go:112](../runs/01-init-backfill__v1-numbered__codex__baseline__run1/code/main.go#L112) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes, based on `go test`, `go vet` and `go build` only. It never ran the tool: the fake API log has no requests during the run. The message doesn't claim a live run.
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__codex__baseline__run1.md](../grades/01-init-backfill__v1-numbered__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
