# Grade: 01-init-backfill__v1-numbered__claude__baseline__run5

Black-box checks by scripts/grade.sh, 2026-09-25T17:33Z. A grading aid; labels come from reading the code.

| Check | Result | Detail |
| --- | --- | --- |
| build | pass |  |
| E1 | pass |  |
| E2 | pass | exit 1; tables created before key check: 0 |
| init exit | pass | exit 0 |
| E3 | pass | customers(created,email,id,name,object) subscriptions(created,customer,id,object,status) |
| E4 | pass | customers=237 (want 237), subscriptions=181 |
| I1 | pass | subscriptions=181 (want 181), canceled=30 (want 30) |
| I2 | pass | 1 x 429 during init, import complete: yes |
| E5 | pass | customers: 237 subscriptions: 181 (tables hold 237 / 181) |
| I3 | pass | second init exit 0; counts 237/181 -> 237/181 |
