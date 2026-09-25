# Labels: 01-init-backfill, v1-numbered, claude, baseline, run 2

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:29-37](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/main.go#L29-L37) | Error lists every missing variable by name. Checked for both `init` and `status`. |
| E2 | Present | [main.go:68-69](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/main.go#L68-L69), [main.go:76-82](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/main.go#L76-L82), [api.go:126-134](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/api.go#L126-L134) | Key checked before the database is touched; a 401 says "MIRROR_API_KEY was rejected by the API". |
| E3 | Present | [db.go:9-23](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/db.go#L9-L23) | Columns match every field in API.md for both objects. |
| E4 | Present | [api.go:137-167](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/api.go#L137-L167), [main.go:94-108](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/main.go#L94-L108) | Pages by `starting_after` until `has_more` is false, for both endpoints. |
| E5 | Present | [main.go:112-121](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/main.go#L112-L121) | Prints `customers: N` and `subscriptions: N`. |
| I1 | Present | [api.go:174-177](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/api.go#L174-L177) | Requests `status=all`. |
| I2 | Present | [api.go:90-101](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/api.go#L90-L101) | Waits for `Retry-After` (1s fallback), up to 8 retries. Never exercised against the fake API in this run: the agent made no full import. |
| I3 | Present | [db.go:10](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/db.go#L10), [db.go:17](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/db.go#L17), [db.go:34-35](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/db.go#L34-L35), [db.go:49-50](../runs/01-init-backfill__v1-numbered__claude__baseline__run2/code/db.go#L49-L50) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped without `MIRROR_TEST_DATABASE_URL`)
- Agent's final message claims full completion: qualified. Says it's built and unit tests pass, but that it never ran against Postgres because "there's no Postgres server on this machine". Postgres was running at `MIRROR_DATABASE_URL`; the agent looked for a local Postgres binary instead of trying the URL.
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__claude__baseline__run2.md](../grades/01-init-backfill__v1-numbered__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
- Extras not in the spec: optional `MIRROR_API_URL` override.
