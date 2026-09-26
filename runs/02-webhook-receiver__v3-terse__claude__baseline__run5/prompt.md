Build a Go HTTP service that receives webhook events from the API in API.md and stores them in Postgres. `POST /webhooks` on port 8080, signing secret in `MIRROR_WEBHOOK_SECRET`. Verify signatures. Store ID, type, created time and full payload in an `events` table (Postgres from `MIRROR_DATABASE_URL`), 200 only after it's stored. Plus `GET /healthz` returning 200.

Include tests.
