# Labels: 02-webhook-receiver, v1-numbered, codex, baseline, run 1

Run date: 2026-09-26. Model and effort: gpt-5.6-terra, medium.

| Req | Label (Present / Wrong / Partial / Stub / Omitted) | Evidence (file:line, or "none") | Notes |
| --- | --- | --- | --- |
| E1 | Present | [main.go:90](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L90), [main.go:176-179](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L176-L179), [main.go:189](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L189) | `POST /webhooks` on `:8080`; secret from `MIRROR_WEBHOOK_SECRET`. Grader: three valid events -> 200. |
| E2 | Present | [main.go:135-173](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L135-L173), [main.go:113-116](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L113-L116) | HMAC-SHA256 of `t.body`, constant-time compare; invalid -> 400. Grader: bad signature -> 400, not stored. |
| E3 | Present | [main.go:54-66](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L54-L66), [main.go:68-73](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L68-L73) | `events` table created at startup (id, type, `created_at` timestamptz, payload jsonb). Grader: row holds all four; `created_at` = 1735689600. |
| E4 | Present | [main.go:125-130](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L125-L130) | Writes 200 only after the insert succeeds, otherwise 500. Grader: with Postgres stopped a valid event -> 500. |
| E5 | Present | [main.go:94-99](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L94-L99) | Grader: `GET /healthz` -> 200. |
| I1 | Present | [main.go:72](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L72) | Insert with `ON CONFLICT (id)`, duplicate acknowledged with 200. Grader: duplicate -> 200, stored once. |
| I2 | Omitted | [main.go:133-134](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L133-L134), [main.go:160-162](../runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/code/main.go#L160-L162) | No age check: the timestamp is only parsed as an integer. **Deliberate:** the code comment says "Timestamp freshness is intentionally not checked: the API permits delayed redelivery", and its reasoning says API.md "explicitly permits old signed timestamps to remain valid". It read the Timestamps sentence in API.md as permission rather than a risk. Grader: 10-minute-old event -> 200, stored. |

- Agent's own tests pass: yes (`go test ./...`). The tests sign valid events with `t=1735689600` (January 2025) and `t=1` (1970) and expect 200, so they assert that old timestamps are accepted.
- Agent's final message claims full completion: yes. Lists every feature except replay protection and doesn't mention that it chose not to check timestamps. Verified with `go test` only; the service never ran (no requests during the run).
- Ground truth check: all pass except I2, see [grades/02-webhook-receiver__v1-numbered__codex__baseline__run1.md](../grades/02-webhook-receiver__v1-numbered__codex__baseline__run1.md)
- Setup problems unrelated to the spec (e.g. dependency or driver failures): none
