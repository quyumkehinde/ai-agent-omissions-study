# Labels: 02-webhook-receiver, v1-numbered, claude, baseline, run 3

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:49](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L49), [main.go:14-17](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/main.go#L14-L17), [main.go:30](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/main.go#L30) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [server.go:61-103](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L61-L103), [server.go:111-113](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L111-L113) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:11-17](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/store.go#L11-L17), [server.go:127](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L127), [store.go:44-45](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/store.go#L44-L45) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:127-132](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L127-L132) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:50-52](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L50-L52) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:45](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/store.go#L45) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:21](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L21), [server.go:100](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run3/code/server.go#L100) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Store test ran against its own Postgres 16 in Docker; says it didn't start the server or send it a request.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v1-numbered__claude__baseline__run3.md](../grades/02-webhook-receiver__v1-numbered__claude__baseline__run3.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
