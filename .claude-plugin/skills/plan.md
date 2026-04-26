---
name: plan
description: Use before writing any code — searches for similar past work, surfaces assumptions, asks scope questions, and confirms the plan. Gate requires developer to reply "approved" before implementation starts. Do not skip for "simple" changes. Not for reading code or answering questions.
user-invocable: true
---

# Plan Before Coding

## Iron Law

**NO CODE BEFORE ASSUMPTIONS ARE STATED, SIMILAR WORK IS FOUND, AND THE PLAN IS CONFIRMED.**

| Rationalization | Why it fails |
|---|---|
| "This is a small change, I'll just implement it directly" | Small API changes still require generation + validation + conversion. Missing one step causes check-generate failure in CI. Phase 6 confirms the 8-step process applies to ALL API changes. |
| "I already know how this should be done from similar repos" | Gardener has repo-specific patterns (DeployWaiter not Helm, ManagedResource not direct apply, Botanist/Flow not simple reconcile). Phase 3 shows reviewers reject non-standard approaches. |
| "The tests can be added later" | Phase 3 top review theme #1: testing approach is the most common blocker. Wrong test approach (gomock vs fake client) causes rework. |

## Red Flags

- Writing any code before stating what packages will be modified and what tests will be run
- Proceeding after "looks good" or "ok" without the word "approved"
- Not searching for similar merged PRs before starting
- Skipping the multi-cluster scope question for changes touching reconcilers

## Steps

### Step 0: Check Repeating Tasks

Before searching, check if this is a **repeating task** documented in AGENTS.md:

- **Drop K8s version**: guide PRs #14615, #14501, #13487
- **Promote feature gate**: guide PRs #14531, #14422, #14145
- **Mock-to-fake migration**: guide PRs #14633, #14569
- **Add new feature gate**: guide PR #14279

If it matches a repeating task, read the guide PR **file list** first:
```bash
gh pr diff [guide-pr-number] --repo gardener/gardener --name-only
```
If the guide PR has **fewer than 50 files**, also read the full diff for patterns:
```bash
gh pr diff [guide-pr-number] --repo gardener/gardener
```
If **50+ files**, do NOT read the full diff — it will exhaust context. Use the file list to plan, then implement file-by-file.

Then state: "This is a repeating task ([type], done N times). Most recent: PR #NNNN. I've read its diff ([N] files). Follow the same pattern?"

### Step 1: Find Similar Work (if not a repeating task)

Search for similar merged PRs:

```bash
gh pr list --repo gardener/gardener --state merged --search "[keywords from task]" --limit 10 --json number,title,files --jq '.[] | {number, title, files: [.files[].path]}'
```

Search for similar closed issues:

```bash
gh issue list --repo gardener/gardener --state closed --search "[keywords]" --limit 10 --json number,title
```

For API changes specifically:
```bash
gh pr list --repo gardener/gardener --state merged --search "pkg/apis" --limit 10 --json number,title,files --jq '.[] | select(.files | map(.path) | any(test("pkg/apis")))'
```

For component changes:
```bash
gh pr list --repo gardener/gardener --state merged --search "pkg/component/[name]" --limit 10 --json number,title
```

For feature gate work:
```bash
gh pr list --repo gardener/gardener --state merged --search "feature gate" --limit 10 --json number,title
```

If a similar PR is found:
1. Read the diff: `gh pr diff [number] --repo gardener/gardener`
2. Read review comments: `gh api repos/gardener/gardener/pulls/[number]/reviews --jq '.[].body'`
3. Present to developer: "I found PR #N — [title]. It changed [files]. Reviewer asked for [feedback]. Follow the same approach?"

If no similar work found: state "No similar merged PRs found." and proceed.

### Step 1b: Blast radius search

After identifying the primary change area, grep for all consumers/callers:

```bash
# Function or type being changed
grep -rn "FunctionName\|TypeName" pkg/ --include="*.go" | grep -v _test.go | grep -v mock | grep -v "zz_generated"

# Error pattern being added or fixed
grep -rn "error message string\|ErrorType" pkg/ --include="*.go" | grep -v _test.go

# For feature gates: search for the GATE NAME as a string (catches docs, examples, indirect usage)
grep -rn "FeatureGateName" pkg/ docs/ example/ --include="*.go" --include="*.yaml" --include="*.md" 2>/dev/null
```

For each call site found, ask: "Does this caller need to change too?" Add any secondary files to the plan.

**For migration tasks** (mock→fake, K8s version drop, deprecated API removal): enumerate ALL target files before starting.
```bash
# Example: mock→fake migration
grep -rln "gomock\|NewController\|MockClient" pkg/ --include="*_test.go" | wc -l
grep -rln "gomock\|NewController\|MockClient" pkg/ --include="*_test.go"

# Example: K8s version drop
grep -rn "k8sGreaterEqual129\|VersionConstraintK8s129" pkg/ --include="*.go" | grep -v _test.go
```

State the total count and full file list. For >10 files, group by package area.

### Step 1c: Ask scope and approach questions

Before finalizing the plan, identify and ask about:

- **Migration scope:** If this is a repeating/migration task, how many files total? List them ALL. Plan processing in batches of 5.

- **Multi-cluster scope:** Which cluster(s) does this target? (garden/seed/shoot)
- **API scope:** Does this touch an external API (core/v1beta1, extensions/v1alpha1, operator/v1alpha1)? If so, all 8 API change steps apply.
- **Test layer:** Does the change impact table require integration tests?
- **Backward compatibility:** Is this additive (new optional field) or a breaking change?
- **Feature gate:** Does this need a feature gate? Which component binaries register it?
- **Component pattern:** New component (DeployWaiter needed) or modifying an existing one?
- **Extension contract:** Does this change `pkg/apis/extensions/v1alpha1`?

**Benchmark mode** (`.benchmark-context` exists): answer all questions from the task spec. Do NOT ask the user. Make the best judgment call from the PR description and linked issue context.

**Normal mode**: Do NOT proceed with unresolved ambiguity when questions can be asked.

### Step 2: Domain checklist (pre-coding)

State which of these apply:

- [ ] Which cluster(s) does this change target? (garden/seed/shoot)
- [ ] Does this touch an external API (core/v1beta1, extensions/v1alpha1, operator/v1alpha1)?
- [ ] If API change: identified ALL 8 checklist steps from `docs/development/changing-the-api.md`?
- [ ] Does this require code generation? (`make generate`)
- [ ] Does this affect a feature gate? Which components register it?
- [ ] Does this affect the extension contract? Will provider extensions need updates?
- [ ] Does this affect `imagevector/containers.yaml`?
- [ ] Does this require RBAC changes? (new API resources accessed = ClusterRole update in charts/)
- [ ] Does this affect the local provider extension? (new extension type = pkg/provider-local/ registration)
- [ ] Are integration tests needed? (check test/integration/ for existing test suites covering this area)
- [ ] Do Skaffold configs need updating? (new dependencies or deployment changes)
- [ ] **Flow dependencies**: Does the PR description mention ordering constraints ("waits for", "after rollout", "before X can use Y")? If yes, grep for all consumers of the changed value and list each downstream task that needs a new dependency.
- [ ] **Shoot flow alignment**: If aligning garden controller with shoot flow patterns, inspect how the shoot controller instantiates the component (`pkg/gardenlet/operation/botanist/`) and replicate the same `components.go` pattern.
- [ ] **Error handling scope**: If fixing an error for one operation type (e.g., List), check if the same error can occur on other operation types (Get, Patch, Delete, Watch) for the same resource. Grep for ALL operations on the affected CRD/resource type.
- [ ] **Security-sensitive paths**: If changing error handling in encryption, credential, or RBAC paths, verify that tolerating errors does not skip security operations. Add safety checks that confirm skipped resources don't require the security operation.

### Step 3: State assumptions with evidence

State what you are assuming:
- Which packages will be modified (be specific: `pkg/gardenlet/controller/shoot/`, not "gardenlet")
- What behavior will change
- What will NOT change
- Which test commands are required (from AGENTS.md change impact table)

Cite any PR Pattern Library matches from AGENTS.md.

### Step 4: Confirm with developer

Present the full plan:
- Similar work reference (or "none found")
- Answered scope questions
- Domain checklist items that apply
- Assumptions
- Affected packages and test plan

**Benchmark mode** (`.benchmark-context` exists in the worktree): auto-approve the plan.
State: "BENCHMARK MODE — auto-approving plan. Proceeding to implement." Then continue immediately.

**Normal mode**: STOP. Wait for the developer to reply "approved" before writing any code.
"Looks good" or "ok" is NOT approval. The word "approved" is required.

*(Skip gate with `--auto-approve-plan`)*

## Handoff

Plan approved → read the relevant skill (api-change, feature-gate, or component if applicable), then invoke implement skill.
