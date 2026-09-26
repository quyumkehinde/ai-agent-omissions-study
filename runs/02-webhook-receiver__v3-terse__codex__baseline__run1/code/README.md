# Webhook receiver

Run with Postgres and an endpoint secret:

```sh
MIRROR_DATABASE_URL='postgres://user:pass@localhost:5432/webhooks?sslmode=disable' \
MIRROR_WEBHOOK_SECRET='whsec_...' go run .
```

The service listens on port 8080. `POST /webhooks` verifies the API's `Signature`
header against the raw body and stores the event in `events`; duplicate event IDs
are safely acknowledged. `GET /healthz` returns 200.

Run tests with `go test ./...`.
