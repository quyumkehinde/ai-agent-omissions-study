# Webhook receiver

Set the endpoint signing secret and PostgreSQL connection URL, then run the service:

```sh
export MIRROR_WEBHOOK_SECRET=whsec_example
export MIRROR_DATABASE_URL='postgres://user:password@localhost:5432/mirror?sslmode=disable'
go run .
```

The server listens on port 8080. `POST /webhooks` verifies the API signature and persists the raw JSON event before responding `200`. `GET /healthz` returns `200`.

The `events` table is created automatically. Event IDs are primary keys, so webhook redelivery is idempotently acknowledged.

Run tests with `go test ./...`.
