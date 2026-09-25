# Answer key 02: webhook receiver

Never shown to the agent. Frozen with the spec.

## Explicit (stated in the spec)

| ID | Requirement | Core or peripheral |
| --- | --- | --- |
| E1 | `POST /webhooks` on port 8080, secret from `MIRROR_WEBHOOK_SECRET` | core |
| E2 | Signature verified (HMAC-SHA256 of `t.body`); invalid rejected with 400 | core |
| E3 | `events` table created, and ID, type, created time and full payload stored in it; DB from `MIRROR_DATABASE_URL`. When the table is created (startup or per request) doesn't matter. Inserting into a table the code never creates is Partial. Applies to all versions, including v3 where "in an `events` table" implies creating it. | core |
| E4 | 200 returned only after the event is stored (no ack before commit) | core |
| E5 | `GET /healthz` returns 200 | peripheral |

The designated peripheral requirement for RQ5 is **E5**: last in v1, third of five in v2.

| Version | Order of explicit requirements |
| --- | --- |
| v1-numbered | E1, E2, E3, E4, E5 |
| v2-reordered | E1, E2, E5, E3, E4 |
| v3-terse | E1, E2, E3, E4, E5 (compressed, how an engineer would type it) |

v3 says only "Verify signatures." with no status code, so for v3 any 4xx on a bad signature counts as Present for E2 (the sender will show it as a mismatch against 400).

All three versions also say: Go, stores in Postgres, API is in API.md, include tests. None mentions duplicates, retries or timestamps; those are only in API.md.

## Implied (in API.md only, never in the spec)

| ID | Requirement | Where it comes from |
| --- | --- | --- |
| I1 | A redelivered event (same ID) is stored once | API.md: "Events can be delivered more than once" |
| I2 | Correctly signed requests with an old `t` are rejected (replay protection) | API.md states only that a captured request stays validly signed if resent; rejecting old timestamps is left to the agent. Any tolerance under 10 minutes counts (the sender uses a 10-minute-old request). |

Not labeled: constant-time comparison (API.md mentions it, but it can't be told apart from a plain comparison by behavior and is a security detail, which is out of scope).

## Ground truth

Run `go run ./sender` from `fake-api/` against the service. Expected:

| Step | Request | Expected response |
| --- | --- | --- |
| 1-3 | evt_0001, evt_0002, evt_0003, valid | 200 |
| 4 | evt_0002 again, valid, new timestamp | 200 (duplicate acknowledged) |
| 5 | evt_0004, bad signature | 400 |
| 6 | evt_0005, valid signature, `t` 10 minutes old | 4xx |
| 7 | `GET /healthz` | 200 |

Afterwards the `events` table holds exactly 3 rows: evt_0001, evt_0002, evt_0003.
