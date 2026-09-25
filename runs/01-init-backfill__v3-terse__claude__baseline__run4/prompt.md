Build a Go CLI `mirror` that copies data from the API in API.md into Postgres. `mirror init` for first-time setup: read `MIRROR_DATABASE_URL` and `MIRROR_API_KEY` (clear error naming whichever is missing), check the key with `GET /v1/account` (clear error if rejected), create `customers` and `subscriptions` tables with every field the API returns, then a backfill. Also a `mirror status` command that prints row counts per table.

Include tests.
