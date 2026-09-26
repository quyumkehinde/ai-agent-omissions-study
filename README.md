# Silent Omissions in AI Coding Agents

Do AI coding agents silently drop requirements? A small, hand-labeled study of 36 agent runs on two Go tasks, with the specs frozen before any run.

## Summary

- Claude Code (Sonnet 5) implemented every requirement in all 30 of its runs, across both tasks and all three wordings of each spec.
- Codex (GPT-5.6-Terra) dropped the same requirement in all 3 of its runs on the webhook task: it never rejected requests with old timestamps, so a captured request could be replayed. Its tests passed and its final message said the work was done every time.
- All 3 omissions were implied requirements (in the API docs, never in the spec). No explicit requirement was dropped in 180 chances.
- Rewording the spec didn't change what got dropped. It changed one detail: when the spec didn't name a status code for a bad signature, all 5 Claude runs chose 401 instead of the 400 the other versions asked for.
- The incident that motivated the study (below) didn't reproduce on a small, self-contained version of the task. That points at what surrounds a task (project size, existing code, session length) rather than how the task is worded. That's the next question.

Write-up: [Do AI coding agents silently drop requirements?](https://medium.com/@quyumkehinde/do-ai-coding-agents-silently-drop-requirements-79faf022f18e). Full counts are in [RESULTS.md](RESULTS.md). Every label links to the code it's based on.

## Motivation

While building [Driftless](https://github.com/quyumkehinde/driftless), a Go service that mirrors Stripe into Postgres, I asked Claude Code for an `init` command that included an initial backfill. It built `init` and silently left out the backfill. I caught it by reading the diff, before committing. Earlier, at BuildQL, where we generated coding lessons from product docs, a user asked for a support feature among several others and got a support button that did nothing.

An omission isn't a bug. A bug is code that does the wrong thing; an omission is a requirement with no code at all, so nothing tests it and nothing fails. I wanted to know how often that happens and what makes it more likely.

## Questions

1. How often does an agent's "done" submission leave out a requirement entirely?
2. Are implied requirements dropped more than explicit ones?
3. Do the agent's own tests or its final message ever reveal an omission?
4. Does rewording the same requirements change which ones get dropped?
5. Is a minor requirement dropped more often in the middle of a list than at the end?

## Design

**Two tasks**, both from Driftless's domain so I can grade every requirement:

- **01, init with backfill:** a CLI whose `init` command checks config and an API key, creates tables and imports every record from a paginated API, plus a `status` command. Modeled on the Driftless incident.
- **02, webhook receiver:** an HTTP service that verifies signed webhook events and stores them in Postgres, plus a health endpoint.

Each has 5 explicit requirements and 2 or 3 implied ones that follow only from the API docs:

- **Spec 01:** the API hides canceled subscriptions unless asked, it rate-limits with 429s, and a setup command gets re-run.
- **Spec 02:** events can be delivered twice, and a signed request stays valid if resent later.

**Three wordings per task**, with identical requirements:

1. **Numbered:** a numbered list, minor requirement last.
2. **Reordered:** the same list with the minor requirement in the middle.
3. **Terse:** compressed the way an engineer would type it ("then a backfill").

**Runs:** Claude Code 5 times per wording (30 runs); Codex once per wording (6 runs, a contrast rather than a head-to-head). Models pinned to `claude-sonnet-5` and `gpt-5.6-terra`, both at medium effort. Neither CLI exposes temperature, so repeated runs measure variation instead.

**Isolation:** each run starts in a fresh directory outside this repo with only the spec and `API.md`, a throwaway config (no memory, project instructions, plugins or MCP servers), no saved sessions and no web search. Postgres and a deterministic fake API are reset for every run. Agents have full shell access, as in normal use.

**Labels:** every requirement in every run is labeled Present, Wrong, Partial, Stub (code that does nothing) or Omitted. Labels are drafted with Claude from the code and the grader's results; I review each one against the code. [`scripts/grade.sh`](scripts/grade.sh) builds and runs each submission against a fresh database and checks each requirement from the outside. For the webhook task it replays valid, duplicate, badly signed and 10-minute-old events, and sends one event with Postgres stopped to check nothing is acknowledged before it's stored.

## Results

| | Runs | Requirements | Present | Omitted |
| --- | --- | --- | --- | --- |
| Spec 01, Claude | 15 | 120 | 120 | 0 |
| Spec 01, Codex | 3 | 24 | 24 | 0 |
| Spec 02, Claude | 15 | 105 | 105 | 0 |
| Spec 02, Codex | 3 | 21 | 18 | 3 |

1. **How often:** 3 of 36 runs left out a requirement, all of them Codex on spec 02. No run was labeled Wrong, Partial or Stub.
2. **Implied vs explicit:** all 3 omissions were implied (87 of 90 present); all 180 explicit requirements were present.
3. **Tests and self-report:** neither caught anything. All three Codex runs passed their own tests, and those tests signed events with timestamps from January 2025 (one also used `t=1`, 1970) and expected them accepted: the tests built the omission in. Their final messages listed features and claimed completion without mentioning replay protection. One run left it out deliberately. Its code comment says "Timestamp freshness is intentionally not checked: the API permits delayed redelivery", but the summary didn't mention it.
4. **Wording:** no effect on omissions (one per wording, all Codex). The only visible effect was the 400 vs 401 choice above.
5. **Position:** the minor requirement (`mirror status`, `/healthz`) was built in all 24 numbered and reordered runs, wherever it sat.

Also seen while labeling:
- 17 of 30 Claude runs never used the database they were given. 9 said there was no Postgres on the machine (it was running at `MIRROR_DATABASE_URL`), and 8 started their own container. Their summaries were upfront about what they hadn't tested.
- In spec 01, Codex's summaries reported unit tests only; the fake API received no requests during any of its runs.

## Limits

- Two tasks, two models, one date each. Results describe these model versions on these tasks, nothing broader.
- Codex has one run per wording, so it shows that the omission happened, not how often.
- I wrote the specs and decided what counts as implied. Freezing the specs before any run limits that bias; it doesn't remove it.
- Implied requirements are only as clear as the docs. The timestamp sentence states a fact ("a request captured in transit stays validly signed if it is sent again later") and leaves the risk to the agent. The deliberate Codex run read it as permission, so that case is arguably a misreading of ambiguous docs. The other two are cleaner.
- One reviewer (me). The grader gives independent evidence for most requirements.
- Go only, small single-session tasks.

## Changes after the freeze

The specs, answer keys and `API.md` never changed after the first commit. These did:

- **First runs discarded:** they used a Claude Code tool allowlist that blocked ordinary work (pipes, heredocs, running the built binary). All runs were redone with full shell access.
- **Grader fixes:** bugs were fixed as they showed up, and every run was regraded after each fix. The grader now:
  - checks each command only for the environment variables it uses;
  - checks that `status` matches the tables;
  - finds the program outside the top-level folder;
  - reads the created time from its own column, not the stored payload.
- **Checklist condition dropped:** the plan had a sixth question, whether asking the agent to map each requirement to its code reduces omissions. With no omissions in spec 01 there was nothing to reduce, so it was dropped and its three trial runs (all requirements present) discarded.

## Related work

- [SWE-RPG](https://arxiv.org/abs/2608.09072) finds implicit requirement recovery is the biggest bottleneck (24.5-46% of failures) on real repository issues. This study is smaller, with hand-written specs so the ground truth per requirement is exact.
- [When Passing Tests Hides Vulnerabilities](https://arxiv.org/abs/2609.10548) finds omission is 48.2% of silent failures in automated security repair, and passing tests don't catch it. The Codex result here is a small from-scratch example of the same pattern.
- [Prompt Variability Effects On LLM Code Generation](https://arxiv.org/abs/2506.10204) (Paleyes et al., Cambridge) shows rewording a prompt changes the structure of generated code, without evaluating correctness. This study asks whether rewording changes which requirements survive; here it didn't.
- [Confident and Wrong](https://arxiv.org/abs/2603.25764) shows agents submit confidently and consistently when wrong. It motivates repeated runs and not trusting "done".
- [Clarity Is Not Assumed](https://arxiv.org/abs/2604.21505), [ClarifyCodeBench](https://arxiv.org/abs/2607.00711), [OctoBench](https://arxiv.org/abs/2601.10343), [Harness-IF](https://arxiv.org/abs/2608.11727) and [HANDBOOK.md](https://arxiv.org/abs/2607.25398) study ambiguity, clarification, process constraints, conflicting instructions and long-horizon rule loss. This study instead holds requirements fixed and short, and asks whether they're dropped.

## Next

- **Load, not wording:** put the same `init` requirement inside an existing codebase with surrounding work and a longer session, the conditions of the original incident.
- **More Codex runs** on spec 02, to see how consistent the timestamp omission is.
- **The checklist intervention,** on tasks where omissions actually occur.

## Reproduce

```
docker compose up -d
scripts/run.sh claude specs/01-init-backfill/v1-numbered.md baseline 1   # or codex
scripts/grade.sh runs/<run-dir>
python3 scripts/summarize.py                                             # rebuilds RESULTS.md
```

| Path | Contents |
| --- | --- |
| `specs/` | the prompts, one file per wording |
| `answer-keys/` | every requirement per spec and the expected result; never shown to agents |
| `fake-api/` | the deterministic billing API, its `API.md`, and a webhook `sender` used for grading |
| `runs/` | each run's prompt, transcript, diff, code and metadata |
| `grades/` | black-box check results per run |
| `labels/` | one labeled table per run |
