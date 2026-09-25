# Silent Omissions in AI Coding Agents: A Small Empirical Study

Status: in progress. Specs are frozen in the first commit, before any agent run.

## Motivation

Building Driftless (a Go service mirroring Stripe into Postgres, github.com/quyumkehinde/driftless) I asked Claude Code to implement an `init` command that included an initial backfill. It shipped `init` alone and silently dropped the backfill. Separately, it omitted a rate limiter until I named it explicitly. Both times the code was correct for a narrower reading of the task, and I only caught the gap by reading the code.

This isn't new to me. At BuildQL (2023-2024) we generated tailored coding lessons from a company's documentation, and output quality was one of the issues that made us stop. The models then sometimes produced incorrect code. That got better, but they still sometimes missed things in the input when generating a lesson. In one, a user building an e-commerce app asked for a support feature alongside several others. The lesson added a support button that did nothing and never built the feature behind it. My guess is that it was dropped because there were a lot of features listed and support looked unimportant, but I never tested that.

This is different from a bug. A bug is code that does the wrong thing. An omission is a requirement that never got translated into any code at all, so there is nothing to test wrong. Nobody writes a test for a feature that doesn't exist. I want to know how often this happens, under what conditions, and whether a cheap process change (asking the agent to check its own output against the spec before declaring done) catches it.

## Related work

I searched for existing benchmarks on requirement coverage, instruction following, and silent failure in LLM code generation. Verified papers, each checked directly on arXiv:

- [Clarity Is Not Assumed](https://arxiv.org/abs/2604.21505) (Orchid benchmark). Studies ambiguity degrading output. My requirements are stated clearly; I'm measuring dropped, not misread, requirements.
- [ClarifyCodeBench](https://arxiv.org/abs/2607.00711). Tests whether models ask clarifying questions on vague specs. My specs are unambiguous on purpose; I test whether stated and implied requirements survive into the diff, not clarification behavior.
- [A Unified Issue Resolution Benchmark for Requirement Clarification, Planning, and Code Generation](https://arxiv.org/abs/2608.09072) (SWE-RPG). Closest prior work: finds "implicit requirement recovery" is the biggest bottleneck (24.5-46% of failures) on real repo issues. I go smaller and narrower, hand-writing specs so ground truth is exact, and test a checklist intervention SWE-RPG doesn't.
- [OctoBench](https://arxiv.org/abs/2601.10343). Measures compliance with structural/process constraints, not functional requirement completeness. Different failure target.
- [Harness-IF](https://arxiv.org/abs/2608.11727). Tests whether agents follow instructions that contradict default behavior. My requirements aren't contradictory, there are just enough of them that some get dropped.
- [HANDBOOK.md](https://arxiv.org/abs/2607.25398). Long policy documents (20-124 pages); finds agents lose rule details over long horizons. My tasks are short, single-session specs; I isolate omission under low context load, not long-horizon drift.
- [When Passing Tests Hides Vulnerabilities](https://arxiv.org/abs/2609.10548). Directly relevant: omission is 48.2% of silent failures in automated repair, and test-passing doesn't catch it. Larger scale (1,030 traces, 7 frameworks), security patches. Mine is small, from-scratch feature building, hand-designed specs instead of mined CVEs.
- [Prompt Variability Effects On LLM Code Generation](https://arxiv.org/abs/2506.10204) (Paleyes, Sendyka, Robinson, Cabrera, Lawrence, Cambridge). Shows that typos, synonyms and paraphrases in a prompt change the structure of the generated code, measured by tree edit distance (TSED), and proposes persona-based prompts to test sensitivity to the user's background. It deliberately does not evaluate correctness. My study measures what it leaves out: whether each requirement survives into the code at all. Running its paraphrase augmentation on my specs, scoring requirement coverage instead of code similarity, is a natural follow-up.
- [Confident and Wrong](https://arxiv.org/abs/2603.25764). Shows agents submit confidently and consistently even when wrong, across repeated runs. Supports running each spec multiple times and not trusting the "done" signal, but doesn't separate omission from other wrongness.

No benchmark I found isolates omission (a requirement with zero corresponding code) from wrongness or ambiguity, at small scale, with hand-verified ground truth per requirement. That gap is the study.

## Research questions

1. On short, multi-requirement specs with a mix of explicit and implied requirements, how often does an agent's first "done" submission omit at least one requirement entirely?
2. Do omissions cluster on implied requirements (never stated) versus explicit ones?
3. Does the agent's own test suite or its "done" self-report ever flag an omission, or does it only ever pass on what it built?
4. Does the way the same requirements are worded (numbered list or terse) change which requirements get omitted? Paleyes et al. show rewording changes the structure of generated code; this asks whether it changes what survives.
5. Is a peripheral requirement dropped more often when it sits mid-list among core ones than when it comes last? (From the BuildQL support-button case.)
6. Does asking the agent to produce a requirement-to-code checklist before finishing reduce omissions?

## Design

Two specs, each written three ways. The requirements and answer key stay fixed across the three versions; only the wording changes. That isolates wording from task, which a set of different specs can't do.

Go only, so language isn't a variable, and it's the language I know best to grade by hand (Terrace, Driftless).

### Specs

- **01, init with backfill:** a CLI `init` command that validates config, creates tables and imports every existing record from a paginated API. Modeled on a Driftless incident where the agent built `init` and silently dropped the backfill. The original prompt was not saved, so this is a new spec written to the same shape, not a reproduction.
- **02, webhook receiver:** a long-running HTTP service that receives and stores signed webhook events.

Different kinds of task (a one-shot CLI job and a long-running service), both from the domain of Driftless, so every requirement is one I've built and can grade.

Each spec has 4-6 explicit requirements, including at least one peripheral one, and 1-3 implied requirements that are never stated but follow from the spec and the API documentation (e.g. "import every subscription" when the API omits canceled ones by default).

### Versions

1. **Numbered:** requirements as a numbered list, peripheral requirement last.
2. **Reordered:** the same list with the peripheral requirement moved to the middle.
3. **Terse:** the same requirements compressed the way an engineer would type them (e.g. "then a backfill"). Fewer words, never fewer requirements.

All versions are written and committed before any run so requirements can't shift after seeing output.

## Repository layout

- `specs/<spec>/v<n>-<name>.md`: the prompts agents receive, one file per version. Frozen (committed) before the first run.
- `answer-keys/`: every requirement per spec, explicit and implied, marked core or peripheral, plus expected ground truth. Never shown to agents.
- `fake-api/`: a small deterministic Stripe-shaped API (Go) with `API.md`, the documentation agents see. Same data every run; list endpoints return a 429 on every 5th request; subscriptions omit canceled ones unless `status=all`, as Stripe does.
- `docker-compose.yml`: Postgres for runs, reset between runs.
- `runs/`: transcripts and generated code per run.
- `labels/`: one rubric table per run (`TEMPLATE.md`).
- `grades/`: black-box check results per run from `scripts/grade.sh` (spec 01): fresh database, missing env vars, wrong key, full import, canceled subscriptions, 429 recovery, `status` output and a second `init`. A grading aid; labels are assigned by reading the code, with these results as supporting evidence.

Run the fake API with `cd fake-api && go run .` (flags: `-429-every`, `-customers`, `-subscriptions`). For spec 02, `go run ./sender` replays a fixed sequence of signed webhooks (valid, duplicate, bad signature, stale timestamp) against a receiver and prints each response next to the expected one. It is a grading aid and is never shown to agents.

## Running

```
docker compose up -d                                         # Postgres (the run script resets it)
scripts/run.sh claude specs/01-init-backfill/v1-numbered.md baseline 1   # or: codex, checklist
```

Each run is isolated so no session can see another's context:

- The workspace is a fresh directory outside this repo containing only `API.md`, so no `CLAUDE.md` or `AGENTS.md` is picked up. It is deleted after the run.
- Claude Code runs from a dedicated, otherwise empty `CLAUDE_CONFIG_DIR` used only for the study, with auto-memory and `CLAUDE.md` loading disabled (`CLAUDE_CODE_DISABLE_AUTO_MEMORY`, `CLAUDE_CODE_DISABLE_CLAUDE_MDS`), plus `--disable-slash-commands`, `--strict-mcp-config` and `--no-session-persistence`.
- Codex runs with a throwaway `CODEX_HOME` holding only credentials, plus `--ephemeral`, `--ignore-user-config` and `--ignore-rules`.
- Postgres and the fake API are restarted per run, so every run sees identical data and the same 429 schedule.
- Agents get unrestricted shell access inside their workspace (Claude Code in `bypassPermissions` mode, Codex in its `workspace-write` sandbox), so they can build, run and test their code the way they normally would. Web search is off for both, so the only inputs are the spec and `API.md`. An earlier harness restricted Claude Code to a short list of commands; it blocked ordinary work (heredocs, pipes, running the built binary), so those runs were discarded.
- Models are pinned: Claude Code uses `claude-sonnet-5`, Codex uses `gpt-5.6-terra`, both at medium reasoning effort. Neither CLI exposes temperature, so runs use each tool's default sampling, as a real user's would. Run-to-run variation is measured directly with 5 runs per version rather than removed.
- `MIRROR_DATABASE_URL` and `MIRROR_API_KEY` are set in the agent's environment, as they would be on a real machine, so specs don't repeat them.

Each run saves `prompt.md`, `transcript.jsonl`, `diff.patch`, the generated `code/`, the fake API's request log and `meta.yaml` (agent version, model, timestamps) under `runs/<spec>__<version>__<agent>__<condition>__run<n>/`. `pilot/` holds a throwaway spec for testing the harness; it isn't part of the study.

## Agents and runs

- **Claude Code** (primary, the incident source and my daily tool): 5 fresh-session runs per version, 2 specs x 3 versions x 5 = 30 runs. Checklist condition on version 1 only, 5 runs per spec = 10 runs.
- **Codex CLI** (contrast, not a head-to-head comparison): 1 run per version = 6 runs.
- 46 runs total. Each task is small (a few thousand tokens of spec plus generated code). Budget under $50; track actual spend.

## Omission vs wrong vs partial: definitions

For each numbered (or identified implied) requirement, after the agent says done, classify:

- **Present**: code exists that implements the requirement and does roughly what it says
- **Wrong**: code exists that addresses the requirement but behaves incorrectly (a bug)
- **Partial**: some but not all of the requirement is implemented
- **Stub**: code exists for the requirement but does nothing (e.g. a button with no behavior behind it, a function that returns without doing the work). Separated from Partial because it hides the gap from a reviewer skimming the output.
- **Omitted**: no code addresses the requirement at all, nothing to point to

Rubric applied by hand, one row per requirement per run, in a spreadsheet or plain markdown table checked into the repo. To catch my own labeling errors, an LLM judge (a single fresh Claude call given the spec and the diff, asked to fill the same rubric) is run on the same rows, and its agreement with my manual labels is reported on a random 20% subset. If agreement is weak, manual labeling stands as ground truth and the mismatch is reported honestly, not smoothed over.

Also recorded per run: whether the agent's own tests pass, and whether the agent's final message claims full completion. This checks whether tests or self-report would have caught the omission on their own.

## Intervention: requirement checklist

On version 1 of each spec, the prompt adds one line: before finishing, list every requirement from the spec and the file/line in the diff that implements it. Compare omission rate with and without this instruction, same agent, same specs, fresh sessions. This is the only intervention tested; it fits the time budget and is the most direct, cheap fix to try first.

## Metrics and honest reporting

This is not statistically powered for strong claims. Report:

- Raw counts: number of omissions / total requirements, broken out by explicit vs implied, by version, by agent, by spec
- Whether the Driftless-shaped spec shows the same failure pattern as the original incident (yes/no, not a rate)
- Omission and stub counts for the peripheral requirement in version 1 (last) vs version 2 (mid-list)
- Omission rate with vs without the checklist intervention, as a count difference, with the caveat that 2 specs x 5 runs is too small to claim a general effect, only a directional one worth a bigger follow-up
- At least 3-5 concrete examples (spec text, diff, and what's missing) shown in full, not just summarized, since the phenomenon is easier to see than to score
- No p-values, no claims beyond what the counts show. If numbers are close, say so.

## Report

Report outline: motivation and incident -> research questions -> method (specs, rubric, agents) -> related work -> results (counts, by version, examples, checklist comparison) -> threats to validity -> what this doesn't show -> future work.

## Threats to validity

- Small n: 2 specs, two agents. No claim generalizes past "this happened this often in this sample." Any effect could be specific to these two tasks; two different kinds of task is the minimum to see whether a pattern repeats.
- I write the specs and the rubric, so my own idea of what counts as "implied" shapes the results. Mitigated by freezing specs before runs and by the LLM-judge cross-check, not eliminated.
- Model versions move fast; results describe specific model versions on specific dates, not agents in general.
- Single rater (me) for most labels; only a subset gets a second opinion (the LLM judge, not a second human).
- Go only. Omission rates may differ in other languages, especially ones with more training data like Python or TypeScript.
- Task domains are ones I already know well, chosen so I can grade accurately, not sampled to represent real-world task diversity.

## Out of scope

- Large-scale statistical claims or leaderboard-style comparison between models
- Security-specific omissions (covered better by existing work, see related work above)
- Long-horizon, multi-session, multi-file repo tasks (this is single-session, small-scope by design)
- Automated omission detection as a shippable tool; the LLM judge here is a labeling aid, not a proposed product
