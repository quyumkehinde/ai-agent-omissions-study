# Labels: 02-webhook-receiver, v3-terse, claude, baseline, run 4

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:28](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L28), [main.go:15-16](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/main.go#L15-L16), [main.go:31](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [signature.go:26-70](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/signature.go#L26-L70), [server.go:40-43](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L40-L43) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [store.go:25-31](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/store.go#L25-L31), [server.go:56](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L56) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:56-67](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L56-L67) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:29-31](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L29-L31) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:55](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/store.go#L55) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:13](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/server.go#L13), [signature.go:67-68](../runs/02-webhook-receiver__v3-terse__claude__baseline__run4/code/signature.go#L67-L68) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 4xx, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: yes. Says it ran the service end to end against its own throwaway Postgres 16 container (not `MIRROR_DATABASE_URL`): valid event -> 200 and stored, duplicate -> 200 with no second row, bad signature -> 401, `/healthz` -> 200.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v3-terse__claude__baseline__run4.md](../grades/02-webhook-receiver__v3-terse__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
