---
name: implement
description: Use after plan is confirmed — implements changes using TDD, runs self-review checklist, and runs pre-commit checks. Not for exploratory changes. Always invoke verify skill before claiming done.
user-invocable: true
---

# Implement with TDD and Self-Review

## Iron Law

**NO IMPLEMENTATION BEFORE THE PLAN IS CONFIRMED AND TEST APPROACH IS DECIDED.**

| Rationalization | Why it fails |
|---|---|
| "Only unit tests are affected, integration tests are unrelated" | Change impact table in AGENTS.md shows cross-layer dependencies. A unit change in API types can break integration tests via conversion and defaulting. |
| "I'll write tests after the implementation is working" | Phase 3 top review theme: testing approach is the most common blocker. Writing tests first surfaces design problems before the wrong design is baked in. |
| "This is a refactor — existing tests cover it" | If existing tests cover it, they pass without your change. If they don't fail WITHOUT your change, you haven't proven your change matters. |

## Red Flags

- Writing any non-test source file as the first change
- Committing with any red test layer
- Skipping pre-commit checks
- Claiming "done" without invoking verify skill
- Creating new files in `third_party/mock/` for new test code
- Missing license header on a new Go file

## Steps

### Step 1: Write failing test first

For bug fixes: write a test that reproduces the bug. Run it. Confirm it fails.

For features: write a test for the expected behavior. Run it. Confirm it fails.

For API changes: follow the api-change skill order (types first). Integration and unit tests come after generation in that workflow.

Use the correct test framework for the package:
- Unit tests: Ginkgo v2 + Gomega in `*_test.go` files in the same package
- Integration tests: `test/integration/` with envtest
- E2e tests: `test/e2e/` with `test/framework/`
- Fake clients: `sigs.k8s.io/controller-runtime/pkg/client/fake` (import alias: `fakeclient`)
- Do NOT create new gomock stubs — check `third_party/mock/` for existing ones, prefer fake clients

### Step 2: Implement the minimum to pass

Write the minimum code to make the failing test pass. No more.

If the plan identified a similar PR, follow the same file change patterns:
```bash
gh pr diff [PR-number] --repo gardener/gardener
```

### Step 3: Self-review checklist (from Phase 3 reviewer standards)

Before claiming implementation is done, check:

- [ ] Flow step names match the actual operation performed (no "DeployIngress" for Istio VirtualService)
- [ ] Function names accurately reflect what they do (no misleading names)
- [ ] Exported functions have godoc comments
- [ ] Error wrapping uses `fmt.Errorf("... %w", err)` not bare error construction
- [ ] No deprecated API usage
- [ ] No constants imported from unrelated packages
- [ ] Test approach: fake clients preferred over gomock for new code (issue #14572)
- [ ] License header on every new Go file: `// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors`
- [ ] No hard-coded container image references (must come from imagevector)
- [ ] If API change: api-change skill checklist completed
- [ ] If feature gate: feature-gate skill checklist completed (all components registered)
- [ ] If new component: component skill checklist completed

### Step 4: Pre-commit checks

Run in order:

1. `make format` — goimports + goimports-reviser
2. `make check` — lint, import restrictions, charts, license, typos
3. `make test` — unit tests (or `go test ./pkg/[path]/...` for targeted run)
4. If API types changed: `make generate && make check-generate`

Do NOT skip step 4 if ANY file in `pkg/apis/` was modified.

### Step 5: Invoke verify skill

Do not claim implementation is done until verify skill confirms all required test layers pass.

## Handoff

Implementation complete and self-review done → invoke verify skill.
Verify passes → invoke submit-pr skill.
