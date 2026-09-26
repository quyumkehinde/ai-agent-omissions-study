# Labels: 02-webhook-receiver, v2-reordered, claude, baseline, run 5

Run date: 2026-09-26. Model and effort: claude-sonnet-5, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [server.go:49](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L49), [main.go:14-17](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/main.go#L14-L17), [main.go:30](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/main.go#L30) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [server.go:61-104](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L61-L104), [server.go:112-115](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L112-L115) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [store.go:10-15](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/store.go#L10-L15), [server.go:129](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L129) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [server.go:129-134](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L129-L134) | Writes 200 only after the insert returns without error, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [server.go:50-52](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L50-L52) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [store.go:38](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/store.go#L38) | `ON CONFLICT (id) DO NOTHING`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Present | [server.go:21](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L21), [server.go:102](../runs/02-webhook-receiver__v2-reordered__claude__baseline__run5/code/server.go#L102) | 5-minute timestamp tolerance. Grader: 10-minute-old event -> 400, not stored. |

- Agent's own tests pass: yes (`go test ./...`, Postgres test skipped)
- Agent's final message claims full completion: qualified. Says "There's no Postgres server on this machine, so the real SQL has never run". Postgres was running at `MIRROR_DATABASE_URL`.
- Ground truth check: all pass, see [grades/02-webhook-receiver__v2-reordered__claude__baseline__run5.md](../grades/02-webhook-receiver__v2-reordered__claude__baseline__run5.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
