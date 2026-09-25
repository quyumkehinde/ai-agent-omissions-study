# Labels: 01-init-backfill, v2-reordered, claude, baseline, run 5

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:32-41](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/main.go#L32-L41) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. Reports one missing variable at a time. |
| E2 | Present | [main.go:62](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/main.go#L62), [api.go:124-126](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/api.go#L124-L126) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [store.go:13-26](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/store.go#L13-L26) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:138-165](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/api.go#L138-L165) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:91](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/main.go#L91) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:172-173](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/api.go#L172-L173) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:96-103](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/api.go#L96-L103) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [store.go:13](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/store.go#L13), [store.go:20](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/store.go#L20), [store.go:40](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/store.go#L40), [store.go:55](../runs/01-init-backfill__v2-reordered__claude__baseline__run5/code/store.go#L55) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Says it is built and tests pass, but that it never ran `init` or `status` against Postgres "because there isn't one on this machine". Postgres was running at `MIRROR_DATABASE_URL`. Second run with this false claim (after v1 run 2).
- Ground truth check: all pass, see [grades/01-init-backfill__v2-reordered__claude__baseline__run5.md](../grades/01-init-backfill__v2-reordered__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
