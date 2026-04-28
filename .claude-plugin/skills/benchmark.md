---
name: benchmark
description: Benchmark plugin skills against real merged or open PRs. The session-start hook pre-fetches PR + issue context into the worktree. This skill reads that context and runs plan → implement → compare automatically with no user prompts.
user-invocable: true
---

# Benchmark Plugin Skills Against Real PRs

## Iron Laws

1. **Context comes from the session-start hook.** Read `$WORKTREE_PATH/.benchmark-context` and `$RESULTS_DIR/task-spec.md`. If either is missing, stop: "Start with `BENCH_PR=<N> claude --dangerouslySkipPermissions`".
2. **Every file operation uses `$WORKTREE_PATH` as root.** Never touch the main repo during implementation.
3. **Never read `gh pr diff` until Step 5.** The task spec is the only input for planning and implementation.
4. **No user prompts.** Auto-approve the plan. Run unattended through all steps.

## Benchmark PR Catalog

| # | PR | Skill(s) tested | Files |
|---|-----|----------------|-------|
| 1 | [#14557](https://github.com/gardener/gardener/pull/14557) | api-change: new field + component wiring | 14 |
| 2 | [#14531](https://github.com/gardener/gardener/pull/14531) | feature-gate: GA promotion + guard removal | 8 |
| 3 | [#14422](https://github.com/gardener/gardener/pull/14422) | feature-gate: Alpha→Beta promotion | 6 |
| 4 | [#14403](https://github.com/gardener/gardener/pull/14403) | api-change: status subresource field | 10 |
| 5 | [#14354](https://github.com/gardener/gardener/pull/14354) | api-change: field removal (2-release-cycle) | 25 |
| 6 | [#14628](https://github.com/gardener/gardener/pull/14628) | component: operator BackupEntry wiring | 21 |
| 7 | [#14569](https://github.com/gardener/gardener/pull/14569) | implement: mock→fake migration | 24 |
| 8 | [#14595](https://github.com/gardener/gardener/pull/14595) | component: scheduler policy change | 17 |
| 9 | [#14623](https://github.com/gardener/gardener/pull/14623) | plan: validation + multi-area change | 7 |
| 10 | [#14588](https://github.com/gardener/gardener/pull/14588) | plan+implement: controller refactor | 6 |

Any open PR also works: `BENCH_PR=14692 claude --dangerouslySkipPermissions`

---

## Steps

### Step 0: Load context

```bash
cat "$WORKTREE_PATH/.benchmark-context"
# Loads: BENCH_PR, WORKTREE_PATH, WORKTREE_BRANCH, RESULTS_DIR, REPO_ROOT, PR_TITLE
```

```bash
cat "$RESULTS_DIR/task-spec.md"
# Contains: PR description + all linked issue bodies
```

If `.benchmark-context` is missing → STOP. Tell user to start with `BENCH_PR=<N> claude --dangerouslySkipPermissions`.

### Step 1: Parse task spec

From `task-spec.md` extract:
- **What** needs to be built (from PR description "What this PR does")
- **Why** it's needed (from linked issue — the root problem)
- **Scope hints** — API group, component names, namespaces, field names mentioned

This is your complete task input. Do NOT look at the actual PR diff.

### Step 2: Plan (worktree-scoped, auto-approved)

Run the full plan skill inside `$WORKTREE_PATH`:

**Step 2a — Check repeating tasks:**
Is this a feature gate promotion, K8s version drop, mock→fake migration, or API field addition?
If yes, follow the guide PR pattern from AGENTS.md.

**Step 2b — Find similar merged PRs:**
```bash
gh pr list --repo gardener/gardener --state merged \
  --search "[keywords from task spec]" \
  --limit 5 --json number,title \
  --jq '.[] | select(.number != ($BENCH_PR | tonumber)) | "#\(.number) \(.title)"'
```

**Step 2c — Domain checklist:**
- Which cluster(s)? (garden/seed/shoot)
- External API touched? → all 8 api-change steps apply
- Feature gate needed?
- `make generate` required?
- RBAC/clusterrole update needed?
- Extension contract affected?
- Peripheral files: docs/, example/, skaffold*.yaml, imagevector?

**Step 2d — State planned file changes:**
List every file you plan to modify with one-line reason.

**Auto-approve.** Save plan to `$RESULTS_DIR/plan.md`. Do NOT wait for user "approved".

### Step 3: Implement (worktree-scoped)

All writes go to `$WORKTREE_PATH`. Run implement skill:

**Step 3a — Write failing test first:**
```bash
cd "$WORKTREE_PATH" && go test ./pkg/<affected-package>/... 2>&1 | tail -5
# Confirm test fails before implementing
```

**Step 3b — Implement minimum to pass:**
Follow the planned file list. Apply Gardener conventions:
- New API fields: pointer + `// +optional` + `omitempty`
- Fake clients not gomock in tests
- License header on new files
- No hard-coded image refs

**Step 3c — Code generation (if API types changed):**
```bash
cd "$WORKTREE_PATH" && make generate
cd "$WORKTREE_PATH" && make check-generate
```

**Step 3d — Pre-commit checks:**
```bash
cd "$WORKTREE_PATH" && make format
cd "$WORKTREE_PATH" && go test ./pkg/<affected-package>/... 2>&1 | tail -20
```

**Record generated files:**
```bash
git -C "$WORKTREE_PATH" diff --name-only > "$RESULTS_DIR/generated-files.txt"
git -C "$WORKTREE_PATH" status --short >> "$RESULTS_DIR/generated-files.txt"
```

### Step 4: Fetch actual PR diff (NOW — not before)

```bash
gh pr diff "$BENCH_PR" --repo gardener/gardener \
  > "$RESULTS_DIR/actual.diff"

gh pr view "$BENCH_PR" --repo gardener/gardener \
  --json files --jq '[.files[].path]' \
  > "$RESULTS_DIR/actual-files.json"
```

### Step 5: Structural comparison

#### 5a. Missed files
```bash
comm -23 \
  <(jq -r '.[]' "$RESULTS_DIR/actual-files.json" | sort) \
  <(grep -v '^?' "$RESULTS_DIR/generated-files.txt" | awk '{print $NF}' | sort) \
  > "$RESULTS_DIR/missed-files.txt"
```

#### 5b. Extra files (false positives)
```bash
comm -13 \
  <(jq -r '.[]' "$RESULTS_DIR/actual-files.json" | sort) \
  <(grep -v '^?' "$RESULTS_DIR/generated-files.txt" | awk '{print $NF}' | sort) \
  > "$RESULTS_DIR/extra-files.txt"
```

#### 5c. Generated artifacts
```bash
diff \
  <(jq -r '.[]' "$RESULTS_DIR/actual-files.json" | grep -E "zz_generated|generated\.pb|openapi_generated" | sort) \
  <(git -C "$WORKTREE_PATH" diff --name-only | grep -E "zz_generated|generated\.pb|openapi_generated" | sort) \
  > "$RESULTS_DIR/generate-diff.txt" || true
```

#### 5d. Pattern checks
```bash
# gomock in new test files
git -C "$WORKTREE_PATH" diff --name-only | grep "_test.go" | while read f; do
  grep -l "gomock\|NewController\|MockClient" "$WORKTREE_PATH/$f" 2>/dev/null \
    && echo "PATTERN_MISMATCH gomock in $f"
done > "$RESULTS_DIR/pattern-checks.txt"
```

#### 5e. Peripheral files
```bash
for p in "charts/gardener/operator/templates/clusterrole.yaml" "imagevector/containers.yaml" "docs/" "example/" "skaffold"; do
  actual=$(jq -r '.[]' "$RESULTS_DIR/actual-files.json" | grep -c "$p" || true)
  generated=$(grep -c "$p" "$RESULTS_DIR/generated-files.txt" 2>/dev/null || true)
  [[ $actual -gt 0 && $generated -eq 0 ]] && echo "MISSED_PERIPHERAL $p"
done > "$RESULTS_DIR/peripheral-checks.txt"
```

### Step 6: Write gap report

```bash
ACTUAL=$(jq length "$RESULTS_DIR/actual-files.json")
GEN=$(git -C "$WORKTREE_PATH" diff --name-only | wc -l | tr -d ' ')
MISSED=$(wc -l < "$RESULTS_DIR/missed-files.txt" | tr -d ' ')
```

#### 6a. Hunk-level comparison for matched files

For files that appear in BOTH actual and generated, compare at the hunk level:
```bash
# For each matched file, extract actual hunks and compare
comm -12 \
  <(jq -r '.[]' "$RESULTS_DIR/actual-files.json" | sort) \
  <(grep -v '^?' "$RESULTS_DIR/generated-files.txt" | awk '{print $NF}' | sort) \
  > "$RESULTS_DIR/matched-files.txt"

TOTAL_HUNKS=0
MATCHED_HUNKS=0
while read -r f; do
  # Count hunks in actual diff for this file
  actual_hunks=$(grep -c "^@@" <(gh pr diff "$BENCH_PR" --repo gardener/gardener -- "$f" 2>/dev/null) || echo 0)
  # Count hunks in generated diff for this file
  gen_hunks=$(git -C "$WORKTREE_PATH" diff -- "$f" 2>/dev/null | grep -c "^@@" || echo 0)
  TOTAL_HUNKS=$((TOTAL_HUNKS + actual_hunks))
  # Rough match: min of actual and generated hunks (conservative)
  match=$((actual_hunks < gen_hunks ? actual_hunks : gen_hunks))
  MATCHED_HUNKS=$((MATCHED_HUNKS + match))
done < "$RESULTS_DIR/matched-files.txt"
```

#### 6b. Weighted scoring

Compute four dimensions:

| Dimension | Weight | Formula |
|-----------|--------|---------|
| Human-authored file coverage | 40% | `(matched files excluding zz_generated) / (actual files excluding zz_generated)` |
| Hunk coverage | 25% | `MATCHED_HUNKS / TOTAL_HUNKS` |
| Pattern correctness | 20% | `1 - (pattern_violations / total_new_test_files)` |
| Peripheral file coverage | 15% | `(matched peripheral files) / (actual peripheral files)` |

```
OVERALL_SCORE = 0.40 * human_file_score + 0.25 * hunk_score + 0.20 * pattern_score + 0.15 * peripheral_score
```

Write `$RESULTS_DIR/gap-report.md` with:
- **Weighted score** (overall + per-dimension breakdown)
- MISSED_FILE list with inferred skill gap cause
- **MISSED_HUNK** list: files where the right file was touched but changes are incomplete (wrong lines, missing logic)
- EXTRA_FILE list
- MISSED_GENERATE list
- MISSED_PERIPHERAL list
- PATTERN_MISMATCH list
- **Skill improvement suggestions**: each gap → target skill file → what could be improved (do NOT edit skill files)

Print summary to terminal:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
BENCHMARK RESULT: PR #N — [title]
Human-authored files : X/Y (N%) [weight 40%]
Hunk coverage        : X/Y (N%) [weight 25%]
Pattern correctness  : X/Y (N%) [weight 20%]
Peripheral files     : X/Y (N%) [weight 15%]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
OVERALL SCORE        : N%
Gap report           : /tmp/benchmark-results/pr-N/gap-report.md
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Step 7: Cleanup worktree

```bash
cd "$REPO_ROOT"
git worktree remove "$WORKTREE_PATH" --force
echo "Worktree removed. Results at: $RESULTS_DIR"
```

---

## Running N Parallel Sessions (Any PR)

```bash
# Open PRs
BENCH_PR=14692 claude --dangerouslySkipPermissions  # api-change
BENCH_PR=14645 claude --dangerouslySkipPermissions  # bug fix
BENCH_PR=14637 claude --dangerouslySkipPermissions  # api-change

# Catalog PRs
BENCH_PR=14557 claude --dangerouslySkipPermissions  # feature + component
BENCH_PR=14531 claude --dangerouslySkipPermissions  # feature gate GA
```

Each session:
1. Hook fetches PR description + linked issue bodies automatically
2. Creates isolated worktree at `/tmp/gardener-benchmark-pr-<N>`
3. Writes `task-spec.md` with full context
4. Claude reads it and runs plan → implement → compare with no prompts

Results accumulate at `/tmp/benchmark-results/pr-<N>/gap-report.md`.

## Handoff

Gap reports accumulate at `/tmp/benchmark-results/pr-<N>/gap-report.md`. Review them manually to decide which skill improvements to apply.
