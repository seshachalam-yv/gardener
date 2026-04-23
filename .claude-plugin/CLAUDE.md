# Gardener Dev Plugin

@.claude/AGENTS.md

## Skills

- **plan** — State assumptions, find similar past work, confirm scope before coding. Gate: reply "approved" before implementing.
- **implement** — TDD with self-review checklist. Pre-commit: make format, make check, make test.
- **verify** — 5-step protocol: identify scope, run tests, read output, confirm green, claim done.
- **review** — Check every changed file against repo's actual review standards from Phase 3.
- **submit-pr** — Build PR description with /area, /kind, release note block. Verify skill must pass first.
- **api-change** — 8-step checklist for API type modifications. Iron Law: no type change without all 8 steps.
- **feature-gate** — Add/promote/remove feature gates with cross-component registration. Unregistered = silently disabled.
- **component** — DeployWaiter interface + ManagedResource pattern. No Helm charts.

Skills are in `.claude/skills/`. Read the relevant skill file before starting that type of work.

## Hooks

- **SessionStart** (`.claude/hooks/session-start.sh`): Orientation with active footguns, available skills, change impact pointer.
- **PreToolUse Edit|Write** (`.claude/hooks/guard-generated-files.sh`): Blocks manual edits to generated files (`zz_generated*.go`, `generated.pb.go`, `openapi_generated.go`, `third_party/mock/`).

## Path-Scoped Rules

Rules in `.claude/rules/` activate when reading matching files:

- `rules/api-types.md` → `pkg/apis/**/*.go`: API field optionality, protobuf tag generation, make generate requirement.
- `rules/component-deployer.md` → `pkg/component/**/*.go`: DeployWaiter interface, ManagedResource, imagevector, component-checklist.md.
- `rules/test-conventions.md` → `**/*_test.go`: Fake clients over gomock, no time.Sleep, Eventually/Consistently.
- `rules/feature-gates.md` → `pkg/features/**/*.go`: Register in every component binary, grep for registration sites.

## Key Invariants

1. Run `make generate` after ANY API type or proto change, then `make check-generate`.
2. New API fields: pointer type + `// +optional` + `omitempty`. All three required.
3. New test code uses `fakeclient`/`fakekubernetes`, not `third_party/mock/` gomock (migration: issue #14572).
4. License header on every new Go file: `// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors`.
5. Feature gates must be registered in every component binary that executes gated code.

## Auto-Approve Flags

- `--auto-approve-plan`: Skip the plan gate (not recommended).
