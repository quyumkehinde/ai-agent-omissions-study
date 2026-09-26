Implemented the Go webhook receiver.

- `POST /webhooks` verifies `Signature` HMACs and stores verified events in Postgres.
- `GET /healthz` returns 200.
- Creates the `events` table on startup using `MIRROR_DATABASE_URL`.
- Requires `MIRROR_WEBHOOK_SECRET`.
- Uses idempotent upserts for duplicate event IDs.
- Returns 200 only after successful storage.
- Added unit tests for health, valid storage, invalid signatures, and storage failures.

Key files: [main.go](/private/tmp/omission-runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/main.go), [webhook.go](/private/tmp/omission-runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/webhook.go), [webhook_test.go](/private/tmp/omission-runs/02-webhook-receiver__v2-reordered__codex__baseline__run1/webhook_test.go).

Verified with `go test ./...`.