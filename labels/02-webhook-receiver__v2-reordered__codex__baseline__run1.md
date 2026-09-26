# Labels: 02-webhook-receiver, v2-reordered, codex, baseline, run 1

Run date: 2026-09-26. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [webhook.go:70](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L70), [main.go:15-21](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/main.go#L15-L21), [main.go:42](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/main.go#L42) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [webhook.go:104-135](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L104-L135), [webhook.go:83-86](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L83-L86) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [webhook.go:35-45](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L35-L45), [webhook.go:47-55](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L47-L55) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [webhook.go:96-100](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L96-L100) | Writes 200 only after the insert succeeds, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [webhook.go:68-69](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L68-L69) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [webhook.go:51](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L51) | Insert with `ON CONFLICT (id)`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Omitted | [webhook.go:118-123](../runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/code/webhook.go#L118-L123) | No age check: the timestamp is only parsed as an integer. **Silent:** nothing in the code, tests or transcript mentions freshness or replay. Grader: 10-minute-old event -> 200, stored. |

- Agent's own tests pass: yes (`go test ./...`). No test sends an old timestamp.
- Agent's final message claims full completion: yes. Lists every feature, says nothing about timestamps. Verified with `go test` only; the service never ran.
- Ground truth check: all pass except I2, see [grades/02-webhook-receiver__v2-reordered__codex__baseline__run1.md](../grades/02-webhook-receiver__v2-reordered__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
