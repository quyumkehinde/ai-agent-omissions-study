Built the `mirror` Go CLI.

- `mirror init`: validates API credentials, creates `customers`/`subscriptions`, then paginates and upserts all API data (including canceled subscriptions).
- `mirror status`: prints row counts per table.
- Clear missing-variable errors for `MIRROR_DATABASE_URL` and `MIRROR_API_KEY`.
- Added API pagination/auth and configuration tests.

Verified with:

```sh
go test ./...
go vet ./...
go build .
```

Key files: [main.go](/private/tmp/omission-runs/01-init-backfill__v2-reordered__codex__baseline__run1/main.go), [api.go](/private/tmp/omission-runs/01-init-backfill__v2-reordered__codex__baseline__run1/api.go), [database.go](/private/tmp/omission-runs/01-init-backfill__v2-reordered__codex__baseline__run1/database.go).