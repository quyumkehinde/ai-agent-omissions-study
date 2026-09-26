# Labels: 02-webhook-receiver, v1-numbered, claude, baseline, run 2

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [webhook.go:46](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L46), [main.go:15-18](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/main.go#L15-L18), [main.go:30](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/main.go#L30) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [webhook.go:53-95](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L53-L95), [webhook.go:104-106](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L104-L106) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:10-15](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/store.go#L10-L15), [webhook.go:118](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L118), [store.go:36-37](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/store.go#L36-L37) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [webhook.go:118-123](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L118-L123) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [webhook.go:47-49](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L47-L49) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:37](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/store.go#L37) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [webhook.go:22](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L22), [webhook.go:53-82](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run2/code/webhook.go#L53-L82) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Tests pass, including a store test against "a real Postgres 16 container" it started itself instead of using `MIRROR_DATABASE_URL`. Says it didn't run the service end to end.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v1-numbered__claude__baseline__run2.md](../grades/02-webhook-receiver__v1-numbered__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
