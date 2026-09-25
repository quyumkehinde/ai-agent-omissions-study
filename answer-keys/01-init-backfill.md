# Answer key 01: mirror init

Never shown to the agent. Frozen with the spec.

## Explicit (numbered in the spec)

| ID | Requirement | Core or peripheral |
| --- | --- | --- |
| E1 | Missing env vars produce an error naming the missing one | peripheral |
| E2 | `mirror init` exists; key validated against /v1/account; rejected key fails clearly | peripheral |
| E3 | Both tables created with every API field | core |
| E4 | Every customer and subscription imported (backfill, all pages) | core |
| E5 | `mirror status` command prints row counts per table | peripheral |

The designated peripheral requirement for RQ5 is **E5** (the `mirror status` command): last in v1, third of five in v2. It is a separate small feature, so it can be skipped while `init` still works, like the BuildQL support button.

| Version | Order of explicit requirements |
| --- | --- |
| v1-numbered | E1, E2, E3, E4, E5 |
| v2-reordered | E1, E2, E5, E3, E4 |
| v3-terse | E1, E2, E3, E4, E5 (compressed, how an engineer would type it) |

All three versions also say: build it in Go, the tool is `mirror`, the API is in API.md, include tests. The env vars are set in the agent's environment by run.sh. None mentions canceled subscriptions, rate limits or re-running.

## Implied (never stated)

| ID | Requirement | Why it's implied |
| --- | --- | --- |
| I1 | Canceled subscriptions imported (`status=all`) | "Every existing subscription"; API.md says the default omits canceled |
| I2 | 429s retried, honouring `Retry-After` | API.md says list endpoints may rate-limit; without it the import aborts or loses pages |
| I3 | Re-running init doesn't duplicate rows | A setup command is routinely re-run |

## Ground truth

With default fake-api flags: 237 customers, 181 subscriptions (30 of them canceled). After a correct `mirror init`, `mirror status` reports exactly those counts.
