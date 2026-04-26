#!/usr/bin/env bash
set -uo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "/Users/I568019/go/src/github.com/gardener/gardener")"
BENCH_PR="${BENCH_PR:-}"

# ── Benchmark session ──────────────────────────────────────────────────────────
if [[ -n "$BENCH_PR" ]]; then
  WORKTREE_PATH="/tmp/gardener-benchmark-pr-${BENCH_PR}"
  WORKTREE_BRANCH="benchmark/pr-${BENCH_PR}-$(date +%Y%m%d-%H%M%S)"
  RESULTS_DIR="/tmp/benchmark-results/pr-${BENCH_PR}"

  echo "Setting up worktree for PR #${BENCH_PR}..."

  # Remove stale worktree for this PR if it exists
  if [[ -d "$WORKTREE_PATH" ]]; then
    git -C "$REPO_ROOT" worktree remove "$WORKTREE_PATH" --force 2>/dev/null || true
    rm -rf "$WORKTREE_PATH"
  fi

  # Fetch and create clean worktree from the PR's base branch
  git -C "$REPO_ROOT" fetch origin --quiet
  PR_BASE=$(gh pr view "$BENCH_PR" --repo gardener/gardener --json baseRefName --jq '.baseRefName' 2>/dev/null || echo "master")
  git -C "$REPO_ROOT" worktree add "$WORKTREE_PATH" "origin/${PR_BASE}" --quiet
  git -C "$WORKTREE_PATH" checkout -b "$WORKTREE_BRANCH" --quiet

  mkdir -p "$RESULTS_DIR"

  # ── Fetch PR context ──
  PR_JSON=$(gh pr view "$BENCH_PR" --repo gardener/gardener \
    --json number,title,body,closingIssuesReferences,labels,files \
    2>/dev/null)

  PR_TITLE=$(echo "$PR_JSON" | jq -r '.title')
  PR_BODY=$(echo "$PR_JSON"  | jq -r '.body')
  PR_FILES=$(echo "$PR_JSON" | jq -r '[.files[].path] | join(", ")')
  ISSUE_NUMBERS=$(echo "$PR_JSON" | jq -r '[.closingIssuesReferences[].number] | join(" ")')

  # ── Fetch linked issue context ──
  ISSUE_CONTEXT=""
  for ISSUE_NUM in $ISSUE_NUMBERS; do
    ISSUE_JSON=$(gh issue view "$ISSUE_NUM" --repo gardener/gardener \
      --json number,title,body 2>/dev/null)
    ISSUE_TITLE=$(echo "$ISSUE_JSON" | jq -r '.title')
    ISSUE_BODY=$(echo "$ISSUE_JSON"  | jq -r '.body')
    ISSUE_CONTEXT="${ISSUE_CONTEXT}
=== Linked Issue #${ISSUE_NUM}: ${ISSUE_TITLE} ===
${ISSUE_BODY}
"
  done

  # ── Fetch reviewer comments (design feedback, requested changes) ──
  REVIEWER_COMMENTS=""
  RAW_REVIEWS=$(gh api "repos/gardener/gardener/pulls/${BENCH_PR}/reviews" \
    --jq '.[] | select(.state == "CHANGES_REQUESTED" or .state == "COMMENTED") | "[\(.user.login)] \(.body)"' 2>/dev/null || true)
  RAW_COMMENTS=$(gh api "repos/gardener/gardener/pulls/${BENCH_PR}/comments" \
    --jq '.[] | "[\(.user.login)] \(.path):\(.line // .original_line // "general") — \(.body)"' 2>/dev/null || true)
  if [[ -n "$RAW_REVIEWS" || -n "$RAW_COMMENTS" ]]; then
    REVIEWER_COMMENTS="
=== Reviewer Feedback ===

--- Review-level comments ---
${RAW_REVIEWS:-"(none)"}

--- Inline comments ---
${RAW_COMMENTS:-"(none)"}
"
  fi

  # ── Write task spec file for Claude to read ──
  cat > "$RESULTS_DIR/task-spec.md" <<TASKSPEC
# Task: PR #${BENCH_PR} — ${PR_TITLE}

## PR Description
${PR_BODY}

## Linked Issues
${ISSUE_CONTEXT:-"No linked issues found."}

## Reviewer Feedback (from actual PR review — use as design hints, not as a solution key)
${REVIEWER_COMMENTS:-"No reviewer comments found."}

## Files changed in actual PR (DO NOT READ UNTIL STEP 6)
${PR_FILES}
TASKSPEC

  # ── Write context file ──
  cat > "$WORKTREE_PATH/.benchmark-context" <<CONTEXT
BENCH_PR=${BENCH_PR}
WORKTREE_PATH=${WORKTREE_PATH}
WORKTREE_BRANCH=${WORKTREE_BRANCH}
RESULTS_DIR=${RESULTS_DIR}
REPO_ROOT=${REPO_ROOT}
PR_TITLE=${PR_TITLE}
CONTEXT

  cat <<BENCHMARK_ORIENTATION
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Gardener Dev Plugin — BENCHMARK SESSION
PR #${BENCH_PR}: ${PR_TITLE}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Worktree : ${WORKTREE_PATH}
Branch   : ${WORKTREE_BRANCH}
Results  : ${RESULTS_DIR}
Task spec: ${RESULTS_DIR}/task-spec.md

Context fetched:
  PR description : ✓
  Linked issues  : $(echo "$ISSUE_NUMBERS" | wc -w | tr -d ' ') found$([ -n "$ISSUE_NUMBERS" ] && echo " (#${ISSUE_NUMBERS})" || echo "")

ALL work happens inside: ${WORKTREE_PATH}
Main repo at ${REPO_ROOT} must NOT be modified.
Running with --dangerouslySkipPermissions — no prompts.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STARTING: Read task spec, then plan → implement → compare
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

BENCHMARK_ORIENTATION

  # ── Inject the task spec as the first user message ──
  # Claude reads this file and immediately starts plan → implement
  cat <<AUTOSTART

Your task is in: ${RESULTS_DIR}/task-spec.md

Read it now, then immediately run the /benchmark skill which will:
1. Parse the task spec (PR description + linked issue)
2. Run /plan inside ${WORKTREE_PATH} — find similar PRs, state assumptions, auto-approve
3. Run /implement inside ${WORKTREE_PATH} — TDD, make generate if needed, make check
4. Compare output to actual PR diff and write gap report to ${RESULTS_DIR}/gap-report.md
5. Apply skill improvements to ${REPO_ROOT}/.claude/skills/

Do not wait for user input. Proceed automatically through all steps.
AUTOSTART

# ── Normal dev session ─────────────────────────────────────────────────────────
else
  cat <<'ORIENTATION'
Gardener Dev Plugin loaded.

Available skills: plan, implement, verify, review, submit-pr, api-change, feature-gate, component, benchmark

Key reminders:
- API changes: 8-step checklist (docs/development/changing-the-api.md). Run make generate after type changes.
- Feature gates: must be registered in EVERY component binary. Unregistered = silently disabled.
- Testing: prefer fake clients over gomock for new tests (migration: issue #14572).
- Components: Go-based DeployWaiter, not Helm charts. ManagedResource for non-control-plane.
- Run make check && make test before committing.

Type /plan to start a new task with assumption checking and similar-work search.

To benchmark the plugin against a real PR (fully automatic, worktree-isolated):
  BENCH_PR=14692 claude --dangerouslySkipPermissions
ORIENTATION
fi

exit 0
