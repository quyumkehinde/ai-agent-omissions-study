# Labels: 02-webhook-receiver, v1-numbered, claude, baseline, run 1

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:36](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L36), [main.go:15-18](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/main.go#L15-L18), [main.go:31](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [handler.go:46-88](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L46-L88), [handler.go:95-98](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L95-L98) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:25-30](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/store.go#L25-L30), [handler.go:111](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L111), [store.go:56-57](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/store.go#L56-L57) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:111-122](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L111-L122) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:37-39](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L37-L39) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:57](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/store.go#L57) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [handler.go:21](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L21), [handler.go:85](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run1/code/handler.go#L85) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Says the Postgres integration test was skipped "because this machine has no Postgres server, so nothing has run against a real database yet". Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v1-numbered__claude__baseline__run1.md](../grades/02-webhook-receiver__v1-numbered__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
