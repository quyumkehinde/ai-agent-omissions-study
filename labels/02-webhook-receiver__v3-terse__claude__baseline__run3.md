# Labels: 02-webhook-receiver, v3-terse, claude, baseline, run 3

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:21](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/handler.go#L21), [main.go:12-13](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/main.go#L12-L13), [main.go:27](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/main.go#L27) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [signature.go:29-73](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/signature.go#L29-L73), [handler.go:33-36](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/handler.go#L33-L36) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [store.go:24-30](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/store.go#L24-L30), [handler.go:49](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/handler.go#L49) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:49-60](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/handler.go#L49-L60) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:22-24](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/handler.go#L22-L24) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:50](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/store.go#L50) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [signature.go:16](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/signature.go#L16), [signature.go:71](../runs/02-webhook-receiver__v3-terse__claude__baseline__run3/code/signature.go#L71) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 4xx, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Store test ran against its own throwaway Postgres 16 container, since stopped; says it didn't run the server end to end.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v3-terse__claude__baseline__run3.md](../grades/02-webhook-receiver__v3-terse__claude__baseline__run3.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
