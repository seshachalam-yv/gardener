---
name: component
description: Use when creating a new Gardener component deployer — enforces DeployWaiter interface, ManagedResource pattern, imagevector, and component-checklist.md. No Helm charts for new components.
user-invocable: true
---

# New Component Deployer

## Iron Law

**NO NEW COMPONENT WITHOUT IMPLEMENTING THE DEPLOYWAITER INTERFACE AND FOLLOWING THE COMPONENT CHECKLIST.**

| Rationalization | Why it fails |
|---|---|
| "I'll use a Helm chart since it's faster" | Phase 6 Non-obvious Convention: Golang components, not Helm charts. Only Istio is excepted (uses Go embed). Reviewers reject new Helm charts. |
| "I don't need ManagedResource, I'll apply directly" | Only shoot control plane components (historic) use direct apply. All seed/shoot system components MUST use ManagedResource for health checks and lifecycle management. |
| "I'll skip the component checklist, it's just guidelines" | The checklist at `docs/development/component-checklist.md` contains hard requirements. Review will reject non-compliant components. |

## Red Flags

- Creating a Helm chart for a new component
- Using `client.Create`/`client.Apply` directly instead of ManagedResource
- Hard-coding container image references
- Creating Secret objects manually instead of using secrets manager
- Not implementing `Wait()`/`WaitCleanup()` methods

## Before Starting

**Check for an existing component FIRST.** Before writing any new code, search for an existing component that already wraps the resource type:
```bash
find pkg/component/ -name "*.go" | xargs grep -l "Interface" | head -20
ls pkg/component/extensions/
```
If a component with `Deploy`/`Destroy`/`Wait`/`WaitCleanup` already exists for the resource type, **REUSE it** by wiring it into `components.go` via `New()`. Do NOT write inline `GetAndCreateOrMergePatch` + `WaitUntilExtensionObjectReady` code when a component abstraction exists.

When reusing an existing component, you typically need ALL of these:
1. **Modify the component's Values/Interface** if new fields needed (e.g., add `SetSecretRef`, `Class` field)
2. **Add it to the `components` struct** in `components.go` (e.g., `backupEntry backupentry.Interface`)
3. **Add a constructor method** (e.g., `newBackupEntry()`) in `components.go`
4. **Call it from `instantiateComponents()`** in `components.go`
5. **Reference `c.backupEntry` in reconcile/delete flows** — do NOT construct objects inline in the reconciler

Read the component checklist:
```bash
cat docs/development/component-checklist.md
```

Find a similar existing component to follow as a template:
```bash
ls pkg/component/
```

## Package Structure

```
pkg/component/[category]/[name]/
    [name].go           # Interface, Values struct, New(), Deploy(), Destroy()
    [name]_test.go      # Tests
    monitoring.go        # Optional: monitoring configuration
    logging.go           # Optional: logging configuration
```

## Required Interface

Implement `component.DeployWaiter`:

```go
type Interface interface {
    component.DeployWaiter
    // Optional: component-specific methods
}
```

Methods:
- `Deploy(ctx context.Context) error` — create/update all managed resources
- `Destroy(ctx context.Context) error` — delete all managed resources
- `Wait(ctx context.Context) error` — wait for deployment to be healthy
- `WaitCleanup(ctx context.Context) error` — wait for destruction to complete

## Constructor Pattern

```go
func New(client client.Client, namespace string, values Values) Interface {
    return &myComponent{
        client:    client,
        namespace: namespace,
        values:    values,
    }
}
```

- Unexported concrete struct, exported `Interface`
- `Values` struct for all configuration
- No hard-coded defaults in constructor

## Key Patterns

### ManagedResource deployment
```go
registry := managedresources.NewRegistry(...)
// Add Kubernetes resources to registry
resources, err := registry.AddAllAndSerialize(deployment, service, configMap, ...)
// Create ManagedResource
return managedresources.CreateForSeedWithLabels(ctx, r.client, r.namespace, name, false, labels, resources)
```

### Image references
```go
image, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameMyComponent)
```
Never hard-code image URIs. Add new images to `imagevector/containers.yaml`.

### Secrets manager
Use the secrets manager for all credential lifecycle. Never create Secret objects manually.

### Unique immutable ConfigMaps/Secrets
Use unique names (with content hash) for immutable ConfigMaps and Secrets. Do not use mutable ConfigMaps with checksum annotations.

## Checklist

- [ ] Read `docs/development/component-checklist.md`
- [ ] Implements `component.DeployWaiter` interface
- [ ] Uses ManagedResource for seed/shoot system components
- [ ] Images from `imagevector/containers.yaml`
- [ ] Secrets via secrets manager
- [ ] Unique immutable ConfigMaps/Secrets
- [ ] Monitoring config in `monitoring.go` (if applicable)
- [ ] Logging config in `logging.go` (if applicable)
- [ ] Unit tests with fake clients
- [ ] License header on all new files

## Ecosystem Wiring (New Extension Types in Operator)

When adding a new extension type (BackupEntry, ContainerRuntime, etc.) to the operator reconciler, ALL of these must be wired:

```bash
# Use this checklist when adding an extension type to pkg/operator/controller/garden/
```

- [ ] **RBAC**: `charts/gardener/operator/templates/clusterrole.yaml` — add verbs for the new API resource
- [ ] **CRD registration**: `pkg/operator/controller/garden/garden/crds.go` — register CRD for the extension type
- [ ] **Component constructor**: add to `components.go` struct + `New()` method + `instantiateComponents()`
- [ ] **Reconcile flow**: reference component in `reconciler_reconcile.go` flow tasks
- [ ] **Delete flow**: reference component in `reconciler_delete.go` flow tasks
- [ ] **Local provider**:
  - `pkg/provider-local/controller/extension/add.go` — register controller
  - `pkg/provider-local/controller/extension/` — add reconciler
  - `pkg/provider-local/charts/` — deployment resources
  - `example/provider-local/garden/base/` — example manifests
- [ ] **Skaffold**: `skaffold-operator.yaml` — if new deployment dependency
- [ ] **Operator util**: `pkg/operator/controller/garden/garden/garden.go` or similar — accessor for garden config
- [ ] **Integration tests**: `test/integration/operator/garden/` — existing test suite covers the new type
- [ ] **Docs**: `docs/concepts/` or `docs/extensions/` — if the extension type is new to the extension contract

Search for the previous extension type wiring as a template:
```bash
# Find how an existing type (e.g., BackupBucket) is wired
grep -rn "BackupBucket\|backupBucket" pkg/operator/ charts/gardener/operator/ pkg/provider-local/ --include="*.go" --include="*.yaml" | grep -v _test.go | head -30
```

## Handoff

Component implementation complete → return to implement skill, then invoke verify skill.
