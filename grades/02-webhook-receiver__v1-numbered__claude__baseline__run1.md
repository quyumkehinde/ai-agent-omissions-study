# Grade: 02-webhook-receiver__v1-numbered__claude__baseline__run1

Black-box checks by scripts/grade.sh, 2026-09-26T02:36Z. A grading aid; labels come from reading the code.

| Check | Result | Detail |
| --- | --- | --- |
| build | pass |  |
| service start | pass |  |
| E5 | pass | GET /healthz -> 200 |
| E1 | pass | valid events -> 200 200 200 |
| E2 | pass | bad signature -> 400 (want 400); stored: 0 |
| E3 | pass | events(id:text,type:text,created_at:timestampwithtimezone,payload:jsonb); evt_0001 row has id, type and payload: yes; created column created_at = 1735689600 (want 1735689600) |
| I1 | pass | duplicate -> 200; evt_0002 rows: 1 (want 1); total rows: 3 |
| I2 | pass | 10-minute-old timestamp -> 400 (want 4xx); stored: 0 |
| E4 | pass | valid event with Postgres stopped -> 500 (want anything but 2xx) |
