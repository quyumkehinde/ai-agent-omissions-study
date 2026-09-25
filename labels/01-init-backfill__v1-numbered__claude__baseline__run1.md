# Labels: 01-init-backfill, v1-numbered, claude, baseline, run 1

Run date: 2026-09-25. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:26-35](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/main.go#L26-L35) | Error lists every missing variable by name. Checked for both `init` and `status`. |
| E2 | Present | [main.go:60](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/main.go#L60), [main.go:69-72](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/main.go#L69-L72), [api.go:115](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/api.go#L115), [api.go:22](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/api.go#L22) | Key checked before the database is touched; 401 gives "API key rejected (401 unauthorized)". |
| E3 | Present | [store.go:12-25](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/store.go#L12-L25) | Columns match every field in API.md for both objects. |
| E4 | Present | [api.go:122-151](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/api.go#L122-L151), [main.go:90-106](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/main.go#L90-L106) | Pages by `starting_after` until `has_more` is false, for both endpoints. |
| E5 | Present | [main.go:110-124](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/main.go#L110-L124) | Prints `customers: N` and `subscriptions: N`. |
| I1 | Present | [api.go:158-160](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/api.go#L158-L160) | Requests `status=all`. |
| I2 | Present | [api.go:87-96](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/api.go#L87-L96) | Waits for `Retry-After` seconds (1s fallback), up to 8 retries. fake-api.log shows two 429s retried. |
| I3 | Present | [store.go:12](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/store.go#L12), [store.go:19](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/store.go#L19), [store.go:51-52](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/store.go#L51-L52), [store.go:61-62](../runs/01-init-backfill__v1-numbered__claude__baseline__run1/code/store.go#L61-L62) | `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO UPDATE`. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped without `MIRROR_TEST_DATABASE_URL`)
- Agent's final message claims full completion: yes
- Ground truth check: all pass, see [grades/01-init-backfill__v1-numbered__claude__baseline__run1.md](../grades/01-init-backfill__v1-numbered__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
- Extras not in the spec: optional `MIRROR_API_URL` override.
