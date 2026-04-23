#!/usr/bin/env bash
set -uo pipefail

cat <<'ORIENTATION'
Gardener Dev Plugin loaded.

Available skills: plan, implement, verify, review, submit-pr, api-change, feature-gate, component

Key reminders:
- API changes: 8-step checklist (docs/development/changing-the-api.md). Run make generate after type changes.
- Feature gates: must be registered in EVERY component binary. Unregistered = silently disabled.
- Testing: prefer fake clients over gomock for new tests (migration: issue #14572).
- Components: Go-based DeployWaiter, not Helm charts. ManagedResource for non-control-plane.
- Run make check && make test before committing.

Type /plan to start a new task with assumption checking and similar-work search.
ORIENTATION

exit 0
