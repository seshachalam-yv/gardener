---
name: verify
description: Use after implementation to confirm all required test layers pass — runs tests, reads actual output, confirms green. Never claim tests pass without running them. Invoke before submit-pr.
user-invocable: true
---

# Verify Before Claiming Done

## Iron Law

**NEVER CLAIM "TESTS PASS" WITHOUT READING THE ACTUAL OUTPUT OF THE TEST COMMAND.**

| Rationalization | Why it fails |
|---|---|
| "Tests should pass based on the changes I made" | Reconcilers, conversion logic, and defaulting interact across packages. A passing unit test does not guarantee passing integration tests. Phase 2 confirms `make verify` is the CI gate. |
| "I ran it earlier and it passed" | Stale results. Code may have changed since the last run. Re-run and re-read. |
| "This is a trivial change so tests aren't needed" | Phase 6 shows even API field additions require regeneration. Trivial changes break `check-generate`. |

## Red Flags

- Saying "tests should pass", "probably works", "I ran it earlier", "tests are likely passing"
- Claiming green without quoting or summarizing actual test output
- Skipping a test layer that the change impact table requires

## Forbidden Phrases

These phrases are NEVER acceptable as verification:
- "should pass"
- "probably works"
- "I ran it earlier"
- "tests are likely passing"
- "this is a trivial change so tests aren't needed"

## Steps

### Step 1: Identify scope from change impact table

Read AGENTS.md change impact table. For each modified path, note which commands must run.

### Step 2: Run required tests

Execute each required command. Common sequences:

**Minimal (any change):**
```bash
make format
make check
make test
```

**API type changes (pkg/apis/):**
```bash
make generate
make check-generate
make check-apidiff
make test
make test-integration
```

**Chart changes:**
```bash
make check
make test
```

**Component changes (pkg/component/):**
```bash
go test ./pkg/component/[name]/...
```

### Step 3: Read actual output

Read the output of each test command. Look for:
- `FAIL` lines
- Non-zero exit codes
- Skipped tests that should have run
- Unexpected warnings

### Step 4: Confirm green

State explicitly what passed:
- "make format: clean (no changes)"
- "make check: passed (exit 0)"
- "make test: 1574 tests passed, 0 failed"
- "make generate && make check-generate: no diff (generated files match)"

### Step 5: Claim done

Only after ALL required test layers show green output, state: "Verification complete. All required checks pass."

If any layer fails: diagnose, fix, re-run from Step 2.

## Handoff

All checks green → invoke submit-pr skill.
Any failure → fix and re-verify.
