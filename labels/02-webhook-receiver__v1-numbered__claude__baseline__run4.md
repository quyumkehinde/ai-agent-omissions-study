# Labels: 02-webhook-receiver, v1-numbered, claude, baseline, run 4

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:50](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L50), [main.go:12-15](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/main.go#L12-L15), [main.go:24](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/main.go#L24) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [server.go:67-104](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L67-L104), [server.go:112-114](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L112-L114) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:9-14](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/store.go#L9-L14), [server.go:126](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L126), [store.go:35-36](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/store.go#L35-L36) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:126-131](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L126-L131) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:51-53](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L51-L53) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:36](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/store.go#L36) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:22](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L22), [server.go:101](../runs/02-webhook-receiver__v1-numbered__claude__baseline__run4/code/server.go#L101) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`)
- Agent's final message claims full completion: qualified. Says the Postgres test was skipped "because there's no Postgres server on this machine" and the insert has not run against a real database. Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v1-numbered__claude__baseline__run4.md](../grades/02-webhook-receiver__v1-numbered__claude__baseline__run4.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
