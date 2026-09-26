Implemented the Go `mirror` CLI.

- `mirror init` validates required env vars, verifies the API key via `/v1/account`, creates both tables, and backfills paginated customers/subscriptions (including canceled subscriptions).
- `mirror status` prints row counts.
- Added idempotent Postgres upserts and 429 retry support.
- Added tests for validation, key rejection, pagination, and canceled-subscription inclusion.

Verified with `go test ./...` and `go build ./...`.