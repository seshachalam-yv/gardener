---
globs: pkg/component/**/*.go
---

# Component Deployer Rules

When modifying files in `pkg/component/`:

1. **New components must implement `component.DeployWaiter`** (Deploy/Destroy/Wait/WaitCleanup). No Helm charts.
2. **Use ManagedResource** for seed/shoot system components. Only historic shoot control plane components use direct apply.
3. **Images from `imagevector/containers.yaml`** only. Never hard-code container image references.
4. **Secrets via secrets manager.** Never create Secret objects manually.
5. **Unique immutable ConfigMaps/Secrets** with content hash names. No mutable ConfigMaps with checksum annotations.
6. **Constructor pattern**: `New()` returns unexported struct via exported `Interface`. `Values` struct for configuration.
7. **Read `docs/development/component-checklist.md`** before creating new components.
