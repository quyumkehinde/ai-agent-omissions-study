# Labels: 02-webhook-receiver, v2-reordered, claude, baseline, run 1

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:48](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L48), [main.go:15-18](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/main.go#L15-L18), [main.go:30](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/main.go#L30) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [handler.go:53-88](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L53-L88), [handler.go:100-103](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L100-L103) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:10-15](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/store.go#L10-L15), [handler.go:114](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L114) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:114-122](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L114-L122) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:45-47](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L45-L47) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:37](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/store.go#L37) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [handler.go:21](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L21), [handler.go:76](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run1/code/handler.go#L76) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped)
- Agent's final message claims full completion: qualified. Says "there's no Postgres on this machine, so the SQL and table creation have never run against a real database". Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v2-reordered__claude__baseline__run1.md](../grades/02-webhook-receiver__v2-reordered__claude__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
