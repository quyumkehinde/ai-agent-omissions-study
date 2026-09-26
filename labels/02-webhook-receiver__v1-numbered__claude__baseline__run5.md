# Labels: 02-webhook-receiver, v1-numbered, claude, baseline, run 5

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:21](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/handler.go#L21), [main.go:15-18](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/main.go#L15-L18), [main.go:31](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [signature.go:29-68](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/signature.go#L29-L68), [handler.go:34-36](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/handler.go#L34-L36) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:22-27](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/store.go#L22-L27), [handler.go:49](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/handler.go#L49), [store.go:49-50](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/store.go#L49-L50) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:49-60](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/handler.go#L49-L60) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:22-24](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/handler.go#L22-L24) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:50](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/store.go#L50) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [signature.go:16](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/signature.go#L16), [signature.go:66](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run5/code/signature.go#L66) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Postgres test ran against a throwaway Postgres 16 container it started itself; says it hasn't run the whole service.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v1-numbered__claude__baseline__run5.md](../grades/02-webhook-receiver__v1-numbered__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
