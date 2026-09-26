# Labels: 02-webhook-receiver, v3-terse, claude, baseline, run 5

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:45](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L45), [main.go:15-16](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/main.go#L15-L16), [main.go:31](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [handler.go:96-130](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L96-L130), [handler.go:63-66](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L63-L66) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [store.go:10-15](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/store.go#L10-L15), [handler.go:79](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L79) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:79-90](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L79-L90) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:46-48](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L46-L48) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:41](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/store.go#L41) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [handler.go:21](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L21), [handler.go:96](../runs/02-webhook-receiver__v3-terse__claude__baseline__run5/code/handler.go#L96) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 4xx, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Postgres test ran against its own throwaway Postgres 16 container, since stopped. Doesn't claim an end-to-end run.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v3-terse__claude__baseline__run5.md](../grades/02-webhook-receiver__v3-terse__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
