# Billing API

A small billing API running locally.

- Base URL: `http://localhost:12111`
- Auth: `Authorization: Bearer sk_test_omission` on every request. A wrong or missing key returns 401.
- Errors: JSON `{"error": {"type": "...", "message": "..."}}`.
- Rate limits: list endpoints may return `429` with a `Retry-After` header (seconds).

## Endpoints

### GET /v1/account
Returns the account: `{"id": "acct_omission", "object": "account", "livemode": false}`.

### GET /v1/customers
Lists customers, oldest first. Query params:
- `limit`: 1-100, default 10
- `starting_after`: a customer ID; returns the page after it

Response: `{"object": "list", "url": "/v1/customers", "has_more": bool, "data": [customer]}`.
Customer: `id`, `object`, `email`, `name`, `created` (unix seconds).

### GET /v1/subscriptions
Lists subscriptions, oldest first. Same `limit` and `starting_after` params, plus:
- `status`: `active`, `past_due`, `trialing`, `canceled`, or `all`. **If omitted, canceled subscriptions are not returned.**

Subscription: `id`, `object`, `customer` (customer ID), `status`, `created` (unix seconds).

## Webhooks

The API sends events to your endpoint as `POST` requests with a JSON body:

```json
{"id": "evt_0001", "object": "event", "type": "customer.created", "created": 1735689600,
 "data": {"object": { ...the customer or subscription... }}}
```

Event types: `customer.created`, `customer.updated`, `subscription.created`, `subscription.updated`, `subscription.deleted`.

**Signatures.** Every request has a `Signature` header of the form `t=<unix seconds>,v1=<hex>`, where `v1` is the HMAC-SHA256 of `<t>.<raw request body>` using your endpoint's signing secret. Compute the same value and compare it to `v1` with a constant-time comparison.

**Timestamps.** `t` is when the request was signed. The signature covers `t`, so a request captured in transit stays validly signed if it is sent again later.

**Delivery.** Events can be delivered more than once, and the same event ID may arrive again after it was acknowledged. Any response other than a `2xx` is treated as a failure and the event is retried later.
