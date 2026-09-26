Built the Go webhook receiver.

- `POST /webhooks` on `:8080`
- HMAC-SHA256 signature verification using `MIRROR_WEBHOOK_SECRET`
- Postgres via `MIRROR_DATABASE_URL`; creates `events` table automatically
- Stores ID, type, UTC created time, and raw JSON payload
- Idempotent duplicate event handling
- Returns 200 only after persistence succeeds
- `GET /healthz` returns 200
- Tests included and passing: `go test ./...`

Key files: [main.go](/private/tmp/omission-runs/02-webhook-receiver__v3-terse__codex__baseline__run1/main.go), [handler.go](/private/tmp/omission-runs/02-webhook-receiver__v3-terse__codex__baseline__run1/handler.go), [handler_test.go](/private/tmp/omission-runs/02-webhook-receiver__v3-terse__codex__baseline__run1/handler_test.go), [README.md](/private/tmp/omission-runs/02-webhook-receiver__v3-terse__codex__baseline__run1/README.md).