---
name: review
description: Use when reviewing code (own or others') — checks every changed file against the repo's actual review standards extracted from Phase 3 PR history. Standalone skill, not part of the plan-implement-verify flow.
user-invocable: true
---

# Code Review Against Repo Standards

## Iron Law

**NEVER APPROVE CODE WITHOUT CHECKING EVERY FILE AGAINST THE REVIEW CHECKLIST.**

| Rationalization | Why it fails |
|---|---|
| "The code compiles and tests pass, so it's fine" | Phase 3 shows the top review blockers are testing approach, naming precision, and architecture patterns — not compilation. Tests passing does not mean code is review-ready. |
| "I wrote it, so I know it's correct" | Self-review misses naming conventions, import ordering, and mock-vs-fake decisions that reviewers catch. Phase 3 documents 5 recurring reviewer themes. |

## Red Flags

- Approving without reading every changed file
- Skipping the footgun checklist for API changes
- Not checking test approach (gomock vs fake client)
- Missing release note block in PR description

## Review Checklist

For EVERY changed file, check:

### 1. Testing approach (Phase 3 theme #1)
- [ ] New test code uses `fakeclient`/`fakekubernetes`, not gomock (migration: #14572)
- [ ] No `time.Sleep()` — use `Eventually`/`Consistently`
- [ ] Table-driven tests (`DescribeTable`/`Entry`) for multiple scenarios
- [ ] Suite file exists with `RegisterFailHandler(Fail)`

### 2. Naming precision (Phase 3 theme #2)
- [ ] Function names accurately describe what they do
- [ ] Flow step names match the actual operation
- [ ] Constants are defined in the package that owns them (no cross-package imports)
- [ ] Variable names are descriptive (no single-letter names except loops/short lambdas)

### 3. Architecture patterns (Phase 3 theme #3)
- [ ] Predicates/mappers used instead of loops for controller reconciliation
- [ ] New components use DeployWaiter interface, not Helm charts
- [ ] ManagedResource for seed/shoot system components
- [ ] Images from imagevector, not hard-coded

### 4. Error handling (Phase 3 theme #4)
- [ ] `fmt.Errorf("failed to <action>: %w", err)` pattern used
- [ ] `apierrors.IsNotFound` check in reconciler Get calls
- [ ] Errors propagated with context, not swallowed
- [ ] No bare `errors.New` where wrapping is appropriate

### 5. Conventions (Phase 3 theme #5)
- [ ] License header on all new files
- [ ] Import ordering: stdlib, external, internal gardener
- [ ] No deprecated API usage
- [ ] Exported functions have godoc comments

## Domain-Specific Review Items

### If API change:
- [ ] 8-step checklist from `docs/development/changing-the-api.md` followed
- [ ] `make generate` was run (check for `zz_generated` file changes)
- [ ] New fields are optional (pointer + `// +optional` + `omitempty`)
- [ ] Protobuf tags generated, not copied
- [ ] Validation tests updated

### If feature gate:
- [ ] Registered in ALL relevant component binaries
- [ ] `grep -r "features.GetFeatures" pkg/ cmd/` confirms registration
- [ ] If GA promotion: `LockToDefault: true` set, conditional checks reviewed

### If component:
- [ ] Implements `component.DeployWaiter` interface
- [ ] Uses ManagedResource (not direct apply) for seed/shoot system components
- [ ] Images from imagevector
- [ ] `docs/development/component-checklist.md` followed

### PR description:
- [ ] `/area` label command present
- [ ] `/kind` label command present
- [ ] Release note block with correct format (category + target group)

## Output

State review findings as:
- **APPROVE**: all checklist items pass
- **REQUEST CHANGES**: list specific items that failed with file:line references

## Handoff

APPROVE → no further action needed.
REQUEST CHANGES → developer addresses findings, then re-invoke review skill.
