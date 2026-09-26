Built the Go webhook receiver with Postgres persistence and tests.

- `POST /webhooks` on `:8080`
- HMAC-SHA256 signature validation using `MIRROR_WEBHOOK_SECRET`
- Creates and writes to an idempotent `events` Postgres table using `MIRROR_DATABASE_URL`
- Returns `200` only after successful storage
- `GET /healthz` returns `200`
- Covers valid delivery, invalid signatures, storage failures, health, and signature verification in tests

Run:

```sh
go test ./...
go run .
```

See [README.md](/tmp/omission-runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/README.md), [main.go](/tmp/omission-runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/main.go), and [main_test.go](/tmp/omission-runs/02-webhook-receiver__v1-numbered__codex__baseline__run1/main_test.go).