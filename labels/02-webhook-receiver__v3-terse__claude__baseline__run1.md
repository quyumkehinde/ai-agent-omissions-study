# Labels: 02-webhook-receiver, v3-terse, claude, baseline, run 1

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [webhook.go:47](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L47), [main.go:15-16](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/main.go#L15-L16), [main.go:30](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/main.go#L30) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [webhook.go:57-97](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L57-L97), [webhook.go:109-112](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L109-L112) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [store.go:10-15](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/store.go#L10-L15), [webhook.go:123](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L123) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [webhook.go:123-134](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L123-L134) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [webhook.go:48-50](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L48-L50) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:39](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/store.go#L39) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [webhook.go:22](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L22), [webhook.go:95](../runs/02-webhook-receiver__v3-terse__claude__baseline__run1/code/webhook.go#L95) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 4xx, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Tests pass, including the store test against the Postgres at `MIRROR_DATABASE_URL` (the first spec 02 run to use it). Says it didn't start the server or send it a webhook.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v3-terse__claude__baseline__run1.md](../grades/02-webhook-receiver__v3-terse__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
