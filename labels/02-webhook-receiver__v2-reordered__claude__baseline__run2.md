# Labels: 02-webhook-receiver, v2-reordered, claude, baseline, run 2

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:49](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L49), [main.go:12-15](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/main.go#L12-L15), [main.go:25](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/main.go#L25) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [handler.go:56-100](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L56-L100), [handler.go:116-119](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L116-L119) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:11-16](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/store.go#L11-L16), [handler.go:135](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L135) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:135-140](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L135-L140) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:46-48](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L46-L48) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:40](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/store.go#L40) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [handler.go:21](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L21), [handler.go:98](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run2/code/handler.go#L98) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped)
- Agent's final message claims full completion: qualified. Says "there's no Postgres server in this environment, so the SQL and table creation haven't run against a real database". Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v2-reordered__claude__baseline__run2.md](../grades/02-webhook-receiver__v2-reordered__claude__baseline__run2.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
