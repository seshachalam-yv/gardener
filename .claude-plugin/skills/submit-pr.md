---
name: submit-pr
description: Use after verify skill confirms all checks pass — builds PR description with required /area, /kind labels and release note block. Never create a PR without verification passing first.
user-invocable: true
---

# Submit Pull Request

## Iron Law

**NEVER CREATE A PR WITHOUT THE VERIFY SKILL CONFIRMING ALL CHECKS PASS.**

| Rationalization | Why it fails |
|---|---|
| "I'll fix the CI failures after creating the PR" | Prow runs `make verify` on every PR. Failures are visible to reviewers and waste CI resources. Fix locally first. |
| "The release note can be added later" | PR template requires release note block. Missing it causes merge label issues. Phase 6 confirms this is a hard requirement. |

## Red Flags

- Creating a PR before verify skill has run
- Missing `/area` or `/kind` label commands
- Missing release note block
- Using wrong release note format

## Steps

### Step 1: Confirm verification

Verify skill must have confirmed all checks pass in the current session. If not, invoke verify skill first.

### Step 2: Build PR description

Use the Gardener PR template format:

```markdown
**What this PR does / why we need it:**
[Description of the change and motivation]

**Which issue(s) this PR fixes:**
Fixes #[issue-number]

**Special notes for your reviewer:**
[Any context the reviewer needs]

**Release note:**
```[category] [target_group]
[Release note text describing the user-visible change]
```
```

**Release note categories:** breaking, noteworthy, feature, bugfix, doc, other
**Target groups:** user, operator, developer, dependency
**Use `NONE` if no release note needed:**
```
```NONE
```
```

### Step 3: Add label commands

Add to PR description body:

```
/area [identifier]
/kind [identifier]
```

**Area identifiers:** audit-logging, auto-scaling, backup, compliance, control-plane-migration, control-plane, cost, delivery, dev-productivity, disaster-recovery, documentation, high-availability, logging, metering, monitoring, networking, open-source, ops-productivity, os, performance, quality, robustness, scalability, security, storage, testing, usability, user-management

**Kind identifiers:** api-change, bug, cleanup, discussion, enhancement, epic, flake, impediment, poc, post-mortem, question, regression, task, technical-debt, test

### Step 4: Push branch to remote

Ensure the current branch is pushed before creating the PR:

```bash
git push -u origin HEAD
```

Do NOT force-push. If the push fails, diagnose and fix the issue.

### Step 5: Create PR

```bash
gh pr create --title "[concise title under 70 chars]" --body "$(cat <<'EOF'
[full PR description from Step 2 + Step 3]
EOF
)"
```

### Step 6: Confirm

Present the PR URL and summary of what was included.

## Handoff

PR created → share the URL with the developer. No further automation steps.
