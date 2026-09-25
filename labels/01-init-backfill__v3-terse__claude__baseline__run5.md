# Labels: 01-init-backfill, v3-terse, claude, baseline, run 5

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:37-48](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L37-L48), [main.go:56](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L56), [main.go:87](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L87) | `init` names each missing variable; `status` requires only `MIRROR_DATABASE_URL`, the one it uses. Grader confirmed. |
| E2 | Present | [main.go:28](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L28), [main.go:64-72](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L64-L72), [api.go:117-121](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/api.go#L117-L121) | Key checked before the database is touched; grader: wrong key exits 1, no tables created. |
| E3 | Present | [db.go:12-25](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/db.go#L12-L25) | Columns match every field in API.md for both objects (grader checked the schema). |
| E4 | Present | [api.go:131-161](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/api.go#L131-L161), [backfill.go:12](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/backfill.go#L12) | Pages until `has_more` is false; grader: 237 customers imported. |
| E5 | Present | [main.go:86](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/main.go#L86) | Grader: prints counts matching the tables. |
| I1 | Present | [api.go:167-169](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/api.go#L167-L169) | Requests `status=all`; grader: 181 subscriptions, 30 canceled. |
| I2 | Present | [api.go:87-94](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/api.go#L87-L94) | Waits for `Retry-After`; grader: import completed through a 429. |
| I3 | Present | [db.go:12](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/db.go#L12), [db.go:19](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/db.go#L19), [db.go:56](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/db.go#L56), [db.go:76](../runs/01-init-backfill__v3-terse__claude__baseline__run5/code/db.go#L76) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`; grader: second `init` leaves counts at 237 / 181. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes, with a caveat. It ran `init` end to end, but instead of using `MIRROR_DATABASE_URL` it started its own Postgres 16 container (first on port 55432, clashing with the provided one, then 55999) and removed it afterwards. Third run that didn't use the database it was given (after v1 run 2 and v2 run 5).
- Ground truth check: all pass, see [grades/01-init-backfill__v3-terse__claude__baseline__run5.md](../grades/01-init-backfill__v3-terse__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
