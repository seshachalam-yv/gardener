---
name: feature-gate
description: Use when adding, promoting, or removing feature gates — enforces cross-component registration to prevent silent failures. Unregistered gates return false with no error. Invoked from plan skill.
user-invocable: true
---

# Feature Gate Workflow

## Iron Law

**NO FEATURE GATE ADDITION WITHOUT REGISTERING IN EVERY COMPONENT BINARY THAT NEEDS IT.**

| Rationalization | Why it fails |
|---|---|
| "I'll register it in the main components and add others later" | If a component doesn't register the gate, `features.DefaultFeatureGate.Enabled(features.X)` returns false silently. No error, no log, no panic. The feature just doesn't work in that binary. |
| "Only gardenlet needs this feature gate" | Feature gates used in API validation (gardener-apiserver) AND controllers (gardenlet) must be registered in both. Shared validation code in `pkg/apis/` executes in whichever binary loads it. |

## Red Flags

- Adding a feature gate constant without adding it to `AllFeatureGates` map
- Registering in only one component binary
- Not grepping for all registration sites
- Promoting to GA without setting `LockToDefault: true`
- Removing a gate before it reaches GA+locked

## Adding a New Feature Gate

### Step 1: Define the constant

File: `pkg/features/features.go`

```go
// MyFeature enables [description].
// owner: @[github-handle]
// alpha: v1.NNN.0
MyFeature featuregate.Feature = "MyFeature"
```

### Step 2: Add to AllFeatureGates map

Same file:

```go
MyFeature: {Default: false, PreRelease: featuregate.Alpha},
```

### Step 3: Register in component binaries

Find all registration sites:
```bash
grep -rn "GetFeatures\|RegisterFeatureGates\|featureGateAccessor" cmd/ pkg/*/features/ --include="*.go" | grep -v _test.go
```

Register in EVERY component binary that executes code gated behind the feature:
- `cmd/gardener-apiserver/` — if the gate affects API validation
- `pkg/gardenlet/features/` — if the gate affects gardenlet reconciliation
- `pkg/operator/features/` — if the gate affects gardener-operator
- `pkg/resourcemanager/features/` — if the gate affects resource-manager
- `cmd/gardener-controller-manager/` — if the gate affects controller-manager
- `cmd/gardener-scheduler/` — if the gate affects scheduler
- `cmd/gardener-admission-controller/` — if the gate affects admission
- `cmd/gardener-node-agent/` — if the gate affects node-agent
- `cmd/gardenadm/` — if the gate affects gardenadm

### Step 4: Add gated code

Use the check:
```go
if features.DefaultFeatureGate.Enabled(features.MyFeature) {
    // gated code
}
```

Identify ALL code paths that need gating. Search broadly for the component/resource being gated:
```bash
grep -rn "componentName\|ComponentName\|component-name\|component_name" pkg/ --include="*.go" | grep -v _test.go | grep -v mock
```

Also search in non-obvious locations:
```bash
grep -rn "componentName" pkg/component/shared/ pkg/gardenlet/controller/seed/seed/components.go dev-setup/ example/ --include="*.go" --include="*.yaml" 2>/dev/null
```

Common locations that need gating:
- **Component deployment**: `pkg/component/<name>/` Deploy/Destroy methods
- **Shared component factories**: `pkg/component/shared/` — shared constructors used by multiple reconcilers
- **Botanist wiring**: `pkg/gardenlet/operation/botanist/`
- **Seed components**: `pkg/gardenlet/controller/seed/seed/components.go`
- **Reconciler flows**: `reconciler_reconcile.go`, `reconciler_delete.go` flow tasks
- **Validation**: `pkg/api/core/validation/` if the gate affects API field semantics
- **Maintenance**: `pkg/controllermanager/controller/shoot/maintenance/` if the gate affects shoot maintenance

### Step 5: Update documentation and release note

## Promoting a Feature Gate

### Alpha → Beta
1. Update `AllFeatureGates`: `Default: true, PreRelease: featuregate.Beta`
2. Update version comment: `// beta: v1.NNN.0`
3. Add release note: `noteworthy operator`

### Beta → GA
1. Update `AllFeatureGates`: `Default: true, PreRelease: featuregate.GA, LockToDefault: true`
2. Update version comment: `// ga: v1.NNN.0`
3. Review all `features.DefaultFeatureGate.Enabled(features.X)` checks — remove them so code executes unconditionally
4. Add release note: `noteworthy operator`

## Removing a Feature Gate

Only remove gates that are GA+locked (`LockToDefault: true`).

1. Remove all `features.DefaultFeatureGate.Enabled(features.X)` conditional checks (code runs unconditionally)
2. Remove constant and map entry from `pkg/features/features.go`
3. Remove registration calls from all component binaries
4. Run `make generate` if any generated code referenced the gate
5. Add release note: `breaking operator` if behavior change

## Handoff

Feature gate work complete → return to implement skill for remaining work, or invoke verify skill.
