# Labels: 02-webhook-receiver, v3-terse, codex, baseline, run 1

Run date: 2026-09-26. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [handler.go:40](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L40), [main.go:17-23](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/main.go#L17-L23), [main.go:40](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/main.go#L40) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [handler.go:71-115](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L71-L115), [handler.go:51-54](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L51-L54) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 401. v3 names no status code, so any 4xx counts. Grader: bad signature -> 401, not stored. |
| E3 | Present | [main.go:53-62](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/main.go#L53-L62), [main.go:64-68](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/main.go#L64-L68) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [handler.go:63-67](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L63-L67) | Writes 200 only after the insert succeeds, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [handler.go:37-39](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L37-L39) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [main.go:68](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/main.go#L68) | Insert with `ON CONFLICT (id)`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Omitted | [handler.go:80-87](../runs/02-webhook-receiver__v3-terse__codex__baseline__run1/code/handler.go#L80-L87) | No age check: the timestamp is only checked to be an integer. **Silent on timestamps:** its reasoning cites "later redelivery", but only to justify idempotent inserts, never freshness. Grader: 10-minute-old event -> 200, stored. |

- Agent's own tests pass: yes (`go test ./...`). The tests sign a valid event with `t=1735689600` (January 2025) and expect 200, so they assert that an old timestamp is accepted.
- Agent's final message claims full completion: yes. Lists every feature and "Tests included and passing", says nothing about timestamps. The service never ran.
- Ground truth check: all pass except I2, see [grades/02-webhook-receiver__v3-terse__codex__baseline__run1.md](../grades/02-webhook-receiver__v3-terse__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
