# Labels: 02-webhook-receiver, v2-reordered, claude, baseline, run 4

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:50](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L50), [main.go:15-18](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/main.go#L15-L18), [main.go:31](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/main.go#L31) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [server.go:57-92](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L57-L92), [server.go:103-106](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L103-L106) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:11-16](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/store.go#L11-L16), [server.go:118](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L118) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:118-123](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L118-L123) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:47-49](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L47-L49) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:40](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/store.go#L40) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:22](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L22), [server.go:87-90](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run4/code/server.go#L87-L90) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped)
- Agent's final message claims full completion: qualified. Says "No Postgres server is installed here, so `TestPostgresStore` skips itself". Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v2-reordered__claude__baseline__run4.md](../grades/02-webhook-receiver__v2-reordered__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
