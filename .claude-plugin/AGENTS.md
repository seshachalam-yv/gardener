# gardener — Domain Intelligence

## Commands
| Action | Command |
|--------|---------|
| Full local verification | `make verify` |
| Extended verification | `make verify-extended` |
| Lint + import restrictions + charts + license + typos | `make check` |
| Format (goimports + goimports-reviser) | `make format` |
| Unit tests | `make test` |
| Unit tests (single package) | `go test ./pkg/[path]/...` |
| Integration tests | `make test-integration` |
| Code generation (protobuf + codegen + manifests) | `make generate` |
| Verify generated files match committed state | `make check-generate` |
| Go mod tidy (3 modules) | `make tidy` |
| Check API incompatible changes | `make check-apidiff` |
| Protobuf generation only | `make generate WHAT="protobuf"` |
| Codegen only (deepcopy, conversion, defaults) | `make generate WHAT="codegen"` |
| Manifests only (CRDs, RBAC) | `make generate WHAT="manifests"` |
| Create local KinD cluster | `make kind-up` |
| Deploy Gardener locally | `make gardener-up` |
| Run e2e tests | `make test-e2e-local` |

## Change Impact Table
| When this changes... | Also run |
|---------------------|----------|
| `pkg/apis/core/` (API types) | `make generate`, `make check-generate`, `make check-apidiff`, `make test`, `make test-integration` |
| `pkg/apis/extensions/` or `pkg/apis/operator/` | `make generate`, `make check-generate`, `make test` |
| `pkg/gardenlet/controller/shoot/` | `make test`, `make test-integration`, `make test-e2e-local` |
| `pkg/component/*/` | `make test` (unit tests for that component package) |
| `pkg/resourcemanager/` | `make test`, `make test-integration` |
| `pkg/operator/` | `make test`, `make test-integration`, `make test-e2e-local-operator` |
| `pkg/operator/controller/garden/` | `make test`, `make test-integration` (operator subset), `make test-e2e-local-operator` |
| `pkg/gardenadm/` | `make test`, `make test-e2e-local-gardenadm-*` |
| `extensions/pkg/` | `make test` |
| `charts/` | `make check`, `make test` |
| `charts/gardener/operator/` | `make check`, verify RBAC ClusterRole if new API resources added |
| `imagevector/containers.yaml` | `make check` (includes `check-skaffold-deps`) |
| `*.proto` files | `make generate WHAT="protobuf"`, `make check-generate` |
| Any API type file (internal or versioned) | `make generate`, `make check-generate`, `make check-apidiff` |
| `pkg/features/features.go` | Grep for all `features.GetFeatures` call sites to verify registration |

## Footguns

- **Protobuf tag reuse is silent and catastrophic.** When removing a field, tombstone the protobuf number with a comment. Reusing the number breaks wire compatibility for all existing clients with no compile-time error. Source: `docs/development/changing-the-api.md` "Removing a Field".
- **Feature gate not registered = silently disabled.** `features.DefaultFeatureGate.Enabled(features.X)` returns `false` if the gate was never registered in that component binary. No error, no log, no panic. Feature simply does not activate.
- **`make generate` before `make check-generate`.** `check-generate` reruns full generation internally and diffs against committed state. CI catches missed generation, but slowly and confusingly. Always run locally after API type or proto changes.
- **New API fields MUST be optional.** Pointer type + `// +optional` comment + `omitempty` JSON tag. Missing any of these three causes validation failures or defaulting issues.
- **Do not copy protobuf tags from other fields.** Run `make generate WHAT="protobuf"`. Copied tags cause wire-format conflicts silently.
- **API field removal is a TWO-release-cycle process.** Step 1: remove code usages. Step 2 (next release): remove field from types. Single-step removal breaks controllers on older versions. Shoot API defaulted fields require THREE release cycles.
- **Use fake clients, not gomock, for new tests.** Active migration tracked in issue #14572. Reviewers request changes if new test code uses `third_party/mock/` instead of `fakeclient`/`fakekubernetes`. Source: Phase 3 review theme #1, PRs #14554, #14569.
- **No hard-coded container image references.** All images must come from `imagevector/containers.yaml`. No Docker Hub or non-IPv6 registry images.
- **GOWORK=off is enforced globally.** Do not create or use `go.work` files.
- **File names must not contain colons.** Enforced by `hack/check-file-names.sh`. Causes `go get` failures.
- **License header required on every Go file.** Format: `// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors` then `// SPDX-License-Identifier: Apache-2.0`. Checked by `make check`.
- **DoNotCopyBackupCredentials feature gate is GA/locked.** Do not gate code behind it. It is `LockToDefault: true`.

## Non-Obvious Conventions

- **Import aliases enforced by golangci-lint `importas` with 80+ rules.** Key aliases: `corev1` for `k8s.io/api/core/v1`, `fakeclient` for `sigs.k8s.io/controller-runtime/pkg/client/fake`, `gardencorev1beta1` for `github.com/gardener/gardener/pkg/apis/core/v1beta1`. Run `make check` to catch violations. Full alias map in `.golangci.yaml.in`.
- **Import restriction boundaries.** `pkg/` cannot import from `cmd/`, `extensions/`, `plugin/`, or `test/`. Enforced by `import-boss` via `make check`.
- **Flow step names must match operations.** Reviewers reject misleading names (e.g., "DeployIngress" when deploying Istio VirtualService). Source: Phase 3, reviewer @rfranzke.
- **Use predicates/mappers over loops in controllers.** Instead of looping namespaces in reconciliation, use predicate filters and mapper functions. Source: Phase 3, reviewer @rfranzke.
- **Golang components, not Helm charts, for new components.** Only Istio is excepted (uses Go embed). Source: `docs/development/component-checklist.md`.

## Testing Anti-Patterns

- **Do NOT use `time.Sleep()` in tests.** Use `Eventually`/`Consistently` from Gomega. Causes flaky tests. Source: Phase 4 (16 kind/flake issues).
- **Do NOT write hand-rolled mocks for interfaces with gomock-generated mocks.** Check `third_party/mock/` first. Prefer fake clients for new code.
- **Do NOT skip `RegisterFailHandler(Fail)` in suite_test.go.** Every Ginkgo suite requires this. Source: Phase 5 (1564 files with this pattern).
- **Test framework for e2e:** `test/framework/` — do not create parallel test infrastructure.
- **Test helpers for integration:** `test/utils/` — check for existing utilities before creating duplicates.

## Domain Patterns

- **Garden/Seed/Shoot cluster model.** Garden cluster: central management plane, managed by `gardener-operator`. Seed cluster: hosts shoot control planes, managed by `gardenlet`. Shoot cluster: end-user Kubernetes cluster. Changes must be placed in the correct cluster context.
- **Botanist/Flow pattern for shoot reconciliation.** Changes to shoot reconciliation in `pkg/gardenlet/controller/shoot/` require understanding the flow dependency graph.
- **DeployWaiter interface.** All components implement `Deploy(ctx)/Destroy(ctx)/Wait(ctx)/WaitCleanup(ctx)`. 80+ implementations in `pkg/component/`.
- **ManagedResource for non-control-plane deployments.** Seed/shoot system components must use ManagedResource for health checks and lifecycle management. Only shoot control plane components (historic) use direct apply.
- **Secrets manager for credential lifecycle.** Never create Secret objects manually. Use the secrets manager abstraction.
- **Imagevector for all container refs.** All image references come from `imagevector/containers.yaml`. Never hard-code image URIs.

### This Repo's Domain-Specific Patterns

- **Extension contract.** `pkg/apis/extensions/v1alpha1` is the contract between Gardener core and all provider extensions. Changes must be backward-compatible and affect all provider implementations.
- **94 reconcilers across 20 API groups.** Before adding a reconciler, search for the owning package in `pkg/gardenlet/`, `pkg/operator/`, `pkg/resourcemanager/`.
- **Aggregated API server (`k8s.io/apiserver`).** Gardener runs its own API server alongside controllers. API types with custom admission go through `plugin/` admission plugins, not webhook handlers.

## Domain Anti-Patterns

- **Wrong mock approach (gomock for new code).** Reviewers request migration to fake clients. Do not introduce new `third_party/mock/` usage. Source: PR #14569, issue #14572.
- **Flow step names not matching operations.** Causes reviewer rejection. "DeployIngress" for an Istio VirtualService is the documented example. Source: Phase 3, reviewer @rfranzke.
- **Importing constants from unrelated packages.** Creates implicit coupling. Reviewers reject cross-package constant imports. Source: Phase 3 reviewer standard.
- **Deprecated API usage.** Checked by linter and reviewers. Source: Phase 3 reviewer standard.

## Multi-Repo Dependencies

- `github.com/gardener/etcd-druid/api v0.36.1`: etcd management CRDs. Changing etcd-druid API types requires updating Gardener's vendored dependency.
- `github.com/gardener/machine-controller-manager v0.61.3`: machine lifecycle management. Worker pool changes may need MCM API updates.
- Extension contract: changes to `pkg/apis/extensions/v1alpha1` affect ALL provider extensions (gardener-extension-provider-aws, -gcp, -azure, etc.). Extension API changes must be backward-compatible.

## Peripheral Files Checklist

After core implementation, check if these also need updates:

- [ ] **RBAC**: `charts/gardener/operator/templates/clusterrole.yaml` — if new API resources are accessed
- [ ] **Skaffold**: `skaffold*.yaml` — if new binary dependencies or deployment changes
- [ ] **Local provider**: `pkg/provider-local/` — if new extension type needs local implementation
- [ ] **CRD charts**: `charts/gardener/operator/files/` — if CRDs changed via `make generate`
- [ ] **Integration tests**: `test/integration/` — check for existing test suites that cover the changed area
- [ ] **E2E tests**: `test/e2e/` — for user-facing behavior changes
- [ ] **Docs**: `docs/concepts/`, `docs/extensions/` — for feature or architecture changes
- [ ] **Example YAML**: `example/` — for API or config changes
- [ ] **Dev setup**: `dev-setup/` — if local development workflow affected

## PR Pattern Library
| Pattern | Last Instance | Files Changed | Key Reviewer Feedback |
|---------|--------------|---------------|----------------------|
| Dependency update (Renovate) | PR #14619 | go.mod, go.sum, imagevector | skip-review label, no substantive review |
| Feature gate promotion | Various | pkg/features/features.go, per-component registration files | Verify registration in all components |
| Flaky test fix | PR #14625 | `*_test.go` | Timeout increases, race condition fixes, Eventually/Consistently usage |
| GEP feature development | PR #14588 | pkg/gardenlet, cmd/gardenadm | Architecture, timeout, flow step naming feedback |
| Istio-native exposure migration | PR #14587 | pkg/component/gardener, pkg/component/observability | Naming conventions, constant placement |
| Mock-to-fake migration | PR #14569 | `*_test.go`, third_party/mock | Test infrastructure modernization, XXL diffs |
| API field addition | (documented) | pkg/apis/[group]/[versions], validation, conversion, defaults | 8-step checklist adherence |
| New component deployer | (documented) | pkg/component/[name]/, imagevector/containers.yaml | DeployWaiter interface, ManagedResource, component-checklist.md |
| K8s version support change | (documented) | supported-kubernetes-versions.yaml, 35+ version-gated files | Multi-file coordination |

## Repeating Tasks

### Drop Kubernetes Version (~every 3 months)

| Instance | PR | Title | Files |
|----------|-----|-------|-------|
| Most recent | [#14615](https://github.com/gardener/gardener/pull/14615) | Drop support for K8s 1.31 | 200 files |
| Previous | [#14501](https://github.com/gardener/gardener/pull/14501) | Drop support for K8s <= 1.30 | 100 files |
| Earlier | [#13487](https://github.com/gardener/gardener/pull/13487) | Drop support for K8s <= 1.29 | 54 files |

To use as guide:
```bash
gh pr diff 14615 --repo gardener/gardener
gh api repos/gardener/gardener/pulls/14615/comments
```
Files that always change: `supported-kubernetes-versions.yaml`, `pkg/utils/version/version.go`, `imagevector/containers.yaml`, `pkg/api/core/validation/shoot.go`, version-gated `pkg/component/` files, OIDC/settings API group (if removed), charts, docs, examples.
Commit sequence: remove version constraints → remove dead API groups → `make generate` → `make verify`.

### Promote Feature Gate (~monthly)

| Instance | PR | Title | Files |
|----------|-----|-------|-------|
| Most recent | [#14531](https://github.com/gardener/gardener/pull/14531) | Promote NewWorkerPoolHash to GA | 8 files |
| Previous | [#14422](https://github.com/gardener/gardener/pull/14422) | Promote UseUnifiedHTTPProxyPort to Beta | 6 files |
| Earlier | [#14145](https://github.com/gardener/gardener/pull/14145) | Promote VPAInPlaceUpdates to Beta | 11 files |

To use as guide:
```bash
gh pr diff 14531 --repo gardener/gardener
```
Files that always change: `pkg/features/features.go`, per-component `features/features.go`, conditional check sites. For GA: remove `Enabled()` guards, add `LockToDefault: true`.

### Mock-to-Fake Migration (ongoing, issue #14572)

| Instance | PR | Title | Files |
|----------|-----|-------|-------|
| Most recent | [#14633](https://github.com/gardener/gardener/pull/14633) | Replace mock clients Part 2 | 27 files |
| Previous | [#14569](https://github.com/gardener/gardener/pull/14569) | Replace mock clients Part 1 | 24 files |

To use as guide:
```bash
gh pr diff 14633 --repo gardener/gardener
```
Pattern: replace `gomock.NewController` + `MockClient` with `fakeclient.NewClientBuilder()` + `interceptor.Funcs` for error injection. Net line reduction expected.

### Add New Feature Gate

| Instance | PR | Title | Files |
|----------|-----|-------|-------|
| Most recent | [#14279](https://github.com/gardener/gardener/pull/14279) | RemoveVali FeatureGate | 15 files |

To use as guide:
```bash
gh pr diff 14279 --repo gardener/gardener
```
Files that always change: `pkg/features/features.go`, per-component registration, gated code paths, docs/deployment/feature_gates.md.
