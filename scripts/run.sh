#!/usr/bin/env bash
# Runs one agent on one spec in a fresh, isolated workspace and saves the
# transcript and generated code under runs/.
#
# usage: scripts/run.sh <claude|codex> specs/<spec>/<version>.md <baseline|checklist> <run-number>
# env:   MODEL (default claude-sonnet-5 / gpt-5.6-terra), EFFORT (default medium), STUDY_CLAUDE_CONFIG_DIR (default ~/.claude-omission-study), RUNS_ROOT (default /tmp/omission-runs)
#
# Isolation: every run starts with no memory of other sessions. The workspace
# lives outside any repo (so no CLAUDE.md / AGENTS.md is discovered), each agent
# gets a throwaway config home (no user settings, memory, history, skills or MCP
# servers), and sessions are not persisted, so a later run can't resume them.
set -euo pipefail

agent=${1:?agent}; spec=${2:?spec file}; condition=${3:?baseline|checklist}; n=${4:?run number}
root=$(cd "$(dirname "$0")/.." && pwd)
spec_id="$(basename "$(dirname "$spec")")__$(basename "$spec" .md)"
run_id="${spec_id}__${agent}__${condition}__run${n}"
out="$root/runs/$run_id"
ws="${RUNS_ROOT:-/tmp/omission-runs}/$run_id"

case "$agent" in
  claude) MODEL=${MODEL:-claude-sonnet-5} ;;
  codex)  MODEL=${MODEL:-gpt-5.6-terra} ;;
esac
EFFORT=${EFFORT:-medium}

[[ -e "$out" ]] && { echo "refusing to overwrite $out" >&2; exit 1; }
[[ "$condition" == baseline || "$condition" == checklist ]] || { echo "bad condition" >&2; exit 1; }
mkdir -p "$out"
rm -rf "$ws"; mkdir -p "$ws"

prompt=$(cat "$spec")
if [[ "$condition" == checklist ]]; then
  prompt+=$'\n\nBefore you finish, list every requirement from this spec and the file and line in your code that implements it.'
fi

# Fresh workspace: only the spec's inputs, in its own git repo so the diff is exact.
cp "$root/fake-api/API.md" "$ws/API.md"
printf '%s\n' "$prompt" > "$out/prompt.md"
git -C "$ws" init -q && git -C "$ws" add -A && git -C "$ws" -c user.name=run -c user.email=run@local commit -qm "run start"
base=$(git -C "$ws" rev-parse HEAD)

# Fresh database and a fresh fake API (its 429 counter starts at zero every run).
(cd "$root" && docker compose down -v >/dev/null 2>&1 && docker compose up -d --wait >/dev/null)
bin=$(mktemp -d)
(cd "$root/fake-api" && go build -o "$bin/fake-api" .)
"$bin/fake-api" >"$out/fake-api.log" 2>&1 &
api_pid=$!
disown $api_pid  # no "Terminated" message when it's killed at exit
trap 'kill $api_pid 2>/dev/null || true; rm -rf "$bin"' EXIT
sleep 1

cfg=$(mktemp -d)
# The tool under test reads these; set them as they would be on a real machine.
export MIRROR_DATABASE_URL="postgres://omission:omission@localhost:55432/omission?sslmode=disable"
export MIRROR_API_KEY="sk_test_omission"
export MIRROR_WEBHOOK_SECRET="whsec_omission"
start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
case "$agent" in
  claude)
    # Subscription auth from a dedicated, study-only config dir (log in once:
    # CLAUDE_CONFIG_DIR=~/.claude-omission-study claude, then /login). It holds
    # no settings, CLAUDE.md or memory; auto-memory and CLAUDE.md loading are
    # also disabled, and sessions aren't saved, so it stays empty between runs.
    (cd "$ws" && CLAUDE_CONFIG_DIR="${STUDY_CLAUDE_CONFIG_DIR:-$HOME/.claude-omission-study}" \
      CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 CLAUDE_CODE_DISABLE_CLAUDE_MDS=1 \
      claude -p "$prompt" \
      --disable-slash-commands --strict-mcp-config --no-session-persistence \
      --model "$MODEL" --effort "$EFFORT" \
      --output-format stream-json --verbose \
      --allowedTools "Read" "Write" "Edit" "Glob" "Grep" \
        "Bash(go *)" "Bash(gofmt *)" "Bash(curl *)" "Bash(psql *)" "Bash(ls *)" "Bash(cat *)" "Bash(mkdir *)" \
      > "$out/transcript.jsonl") || echo "agent exited nonzero" >&2
    ;;
  codex)
    # Copy only credentials into the throwaway CODEX_HOME: no global AGENTS.md,
    # config, rules or history.
    cp "${CODEX_HOME:-$HOME/.codex}/auth.json" "$cfg/auth.json"
    CODEX_HOME="$cfg" codex exec \
      --ephemeral --ignore-user-config --ignore-rules --skip-git-repo-check \
      -C "$ws" -s workspace-write -c sandbox_workspace_write.network_access=true \
      -c shell_environment_policy.ignore_default_excludes=true \
      -m "$MODEL" -c model_reasoning_effort="$EFFORT" \
      --json -o "$out/last-message.md" "$prompt" \
      > "$out/transcript.jsonl" || echo "agent exited nonzero" >&2
    ;;
  *) echo "unknown agent: $agent" >&2; exit 1 ;;
esac
rm -rf "$cfg"

# Save the generated code and the exact diff.
git -C "$ws" add -A
git -C "$ws" -c user.name=run -c user.email=run@local commit -q --allow-empty -m "agent output"
git -C "$ws" diff "$base" HEAD > "$out/diff.patch"
rsync -a --exclude .git "$ws/" "$out/code/"
# Delete the workspace so no later run can stumble on it.
rm -rf "$ws"

{
  echo "run_id: $run_id"
  echo "agent: $agent"
  echo "agent_version: $($agent --version 2>&1 | head -1)"
  echo "model: $MODEL"
  echo "effort: $EFFORT"
  echo "condition: $condition"
  echo "started: $start"
  echo "finished: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "$out/meta.yaml"

echo "saved $out"
