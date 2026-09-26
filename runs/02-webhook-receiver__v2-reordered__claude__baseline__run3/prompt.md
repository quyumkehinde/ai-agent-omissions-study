Build a Go HTTP service that receives webhook events from the API described in API.md and stores them in Postgres.

It must:

1. Accept events at `POST /webhooks` on port 8080, using the signing secret in `MIRROR_WEBHOOK_SECRET`.
2. Verify each request's signature and reject invalid ones with a 400.
3. Serve `GET /healthz`, returning 200.
4. Create an `events` table in Postgres and store each event's ID, type, created time and full payload in it, reading the connection string from `MIRROR_DATABASE_URL`.
5. Respond with 200 only after the event is stored.

Include tests.
