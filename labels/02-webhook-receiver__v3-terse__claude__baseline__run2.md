# Labels: 02-webhook-receiver, v3-terse, claude, baseline, run 2

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:38](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L38), [main.go:15-16](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/main.go#L15-L16), [main.go:31](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [server.go:45-85](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L45-L85), [server.go:93-96](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L93-L96) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [store.go:24-30](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/store.go#L24-L30), [server.go:109](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L109) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:109-120](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L109-L120) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:35-37](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L35-L37) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:56](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/store.go#L56) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:20](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L20), [server.go:83](../runs/02-webhook-receiver__v3-terse__claude__baseline__run2/code/server.go#L83) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 4xx, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Store test ran against a Postgres 16 container it started itself, not `MIRROR_DATABASE_URL`. Doesn't claim an end-to-end run.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v3-terse__claude__baseline__run2.md](../grades/02-webhook-receiver__v3-terse__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
