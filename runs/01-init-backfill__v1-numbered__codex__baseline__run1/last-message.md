Built the Go `mirror` CLI.

- `mirror init` validates required environment variables, checks `/v1/account`, creates Postgres tables, and imports all paginated customers and subscriptions (including canceled subscriptions via `status=all`).
- Imports are idempotent via Postgres upserts.
- `mirror status` prints row counts for both tables.
- Handles API rate limits using `Retry-After`.
- Added unit tests for pagination, canceled-subscription handling, rate-limit retry, and missing configuration.

Validated with:

```sh
go test ./...
go vet ./...
go build ./...
```

Key files: [main.go](/tmp/omission-runs/01-init-backfill__v1-numbered__codex__baseline__run1/main.go), [api.go](/tmp/omission-runs/01-init-backfill__v1-numbered__codex__baseline__run1/api.go), [api_test.go](/tmp/omission-runs/01-init-backfill__v1-numbered__codex__baseline__run1/api_test.go).