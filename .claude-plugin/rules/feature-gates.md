---
globs: pkg/features/**/*.go
---

# Feature Gate Rules

When modifying feature gate files:

1. **Register in EVERY component binary** that executes gated code. Unregistered gates return `false` silently — no error, no log.
2. **Find all registration sites**: `grep -rn "GetFeatures\|RegisterFeatureGates" cmd/ pkg/*/features/ --include="*.go"`
3. **New gates**: define constant in `pkg/features/features.go`, add to `AllFeatureGates` map with `{Default: false, PreRelease: featuregate.Alpha}`.
4. **Promotion**: alpha→beta changes `Default: true`. Beta→GA adds `LockToDefault: true`. GA+locked gates should have conditional checks removed.
5. **Removal**: only remove GA+locked gates. Remove constant, map entry, and ALL registration calls across component binaries.
6. **Shared validation code** in `pkg/apis/` executes in whichever binary loads it — gates used in validation must be registered in gardener-apiserver too.
