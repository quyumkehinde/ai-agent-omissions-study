Build a Go command-line tool called `mirror` that copies data from the API described in API.md into Postgres.

It must:

1. Read the Postgres connection string from `MIRROR_DATABASE_URL` and the API key from `MIRROR_API_KEY`, and exit with a clear error naming whichever is missing.
2. Have a `mirror init` command for first-time setup that checks the API key by calling `GET /v1/account`, and exits with a clear error if the key is rejected.
3. In `mirror init`, create the tables `customers` and `subscriptions` in Postgres, with a column for every field the API returns.
4. In `mirror init`, import every existing customer and subscription from the API into those tables.
5. Have a `mirror status` command that prints how many rows are in each table.

Include tests.
