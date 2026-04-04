# Design: Replacing Gardener's Node Critical Components Controller with Kubernetes Node Readiness Controller

**Issue:** [gardener/gardener#13977](https://github.com/gardener/gardener/issues/13977)
**Date:** 2026-04-04
**Author:** Seshachalam Yerasala Venkata
**Approach:** Gradual replacement via three phases (investigation → bridge → full replacement)

---

## 1. Background & Motivation

Gardener's `node-critical-components` controller (in `gardener-resource-manager`) applies a `NoSchedule` taint to newly created nodes and removes it only once all node-critical components (DaemonSets, pods, CSI drivers) are ready. This prevents workloads from landing on nodes before their infrastructure dependencies are satisfied.

The upstream Kubernetes project recently released the [Node Readiness Controller (NRC)](https://github.com/kubernetes-sigs/node-readiness-controller), which solves the same class of problem declaratively via a `NodeReadinessRule` CRD. Replacing Gardener's bespoke controller with the upstream one reduces maintenance burden and increases upstream involvement.

**Goals:**
- Produce a feature comparison and gap analysis between the two controllers
- Adopt NRC in Gardener via a phased approach that maintains stability
- Contribute missing features (e.g., CSI awareness) upstream where appropriate
- Establish early Gardener presence in the NRC/SIG-Node community

---

## 2. Feature Comparison

### Gardener Node Critical Components Controller

| Property | Detail |
|----------|--------|
| Location | `pkg/resourcemanager/controller/node/criticalcomponents/` |
| Runs in | Seed cluster (resource-manager), watches shoot cluster via delegated client |
| Taint managed | `node.gardener.cloud/critical-components-not-ready:NoSchedule` |
| Configuration | Labels/annotations on existing objects (no CRD) |
| Check mechanism | Directly polls Kubernetes API: pod readiness + DaemonSet scheduling + CSINode drivers |
| Enforcement mode | Bootstrap-only (taint removed once; never re-added) |
| Node targeting | All nodes (no selector) |
| CSI awareness | Yes — checks `CSINode.Spec.Drivers` via annotation `node.gardener.cloud/wait-for-csi-node-<name>` |
| Dry run | No |
| Observability | Warning events on node |
| Maturity | Battle-tested, production |

### Upstream Node Readiness Controller (NRC)

| Property | Detail |
|----------|--------|
| Repo | `github.com/kubernetes-sigs/node-readiness-controller` |
| Runs in | Inside shoot/target cluster (standard Kubernetes controller) |
| Taint managed | Configurable; key must start with `readiness.k8s.io/` |
| Configuration | `NodeReadinessRule` CRD (cluster-scoped) |
| Check mechanism | Reacts to node conditions set by external agents; does NOT check pods/CSI directly |
| Enforcement modes | `bootstrap-only` AND `continuous` |
| Node targeting | Per-rule `nodeSelector` (label selector) |
| CSI awareness | No — must be expressed as a node condition via a condition reporter |
| Dry run | Yes — simulates impact, writes `DryRunResults` to rule status |
| Observability | Per-node `NodeEvaluation` in rule status; Prometheus metrics |
| Maturity | v0.1.1 (alpha), KEP-5233 in progress |

### Gap Analysis

| Feature | Gardener Controller | NRC | Gap / Action Required |
|---------|--------------------|----|----------------------|
| Pod readiness checks | ✅ Native | ❌ Via condition reporter | Need condition reporter (node-agent or custom DaemonSet) |
| CSI driver awareness | ✅ Native | ❌ Not supported | Upstream contribution OR custom condition reporter |
| DaemonSet scheduling checks | ✅ Native | ❌ Via condition reporter | Need condition reporter |
| `bootstrap-only` enforcement | ✅ | ✅ | No gap |
| `continuous` enforcement | ❌ | ✅ | NRC is strictly a superset |
| Configurable taint key | ❌ Fixed | ✅ (`readiness.k8s.io/` prefix required) | Taint key prefix constraint — Gardener uses `node.gardener.cloud/` |
| Per-node-pool targeting | ❌ | ✅ | NRC is strictly better |
| Dry run | ❌ | ✅ | NRC is strictly better |
| Validation webhook | ❌ | ✅ | NRC is strictly better |
| Declarative CRD config | ❌ | ✅ | NRC is strictly better |
| Runs in seed (remote watch) | ✅ | ❌ Must run in shoot | Architecture difference — see Phase 1 |

### Critical Gaps to Address Before Full Replacement

1. **Taint key prefix constraint** — NRC requires `readiness.k8s.io/` prefix; Gardener uses `node.gardener.cloud/`. Either: (a) migrate the Gardener taint key (breaking change, needs care), or (b) contribute relaxed prefix validation upstream.
2. **No native pod/CSI checks** — NRC only reacts to node conditions. Something must write those conditions. Options: gardener-node-agent, a new condition-reporter DaemonSet, or keeping the resource-manager logic as a "condition writer".
3. **Alpha maturity** — NRC is v0.1.1. Gardener should deploy it with a feature gate and maintain a fallback path until NRC reaches stable.

---

## 3. Architecture

### Current Architecture

```
Seed Cluster
└── gardener-resource-manager
    └── node-critical-components controller
        ├── Watches: Node objects in shoot cluster
        ├── Checks: DaemonSet scheduling, Pod readiness, CSINode drivers
        └── Action: Remove taint node.gardener.cloud/critical-components-not-ready
```

### Phase 2 Bridge Architecture (Recommended Next Step)

```
Seed Cluster
└── gardener-resource-manager
    └── node-critical-components controller (modified)
        ├── Watches: Node objects in shoot cluster
        ├── Checks: DaemonSet scheduling, Pod readiness, CSINode drivers (unchanged)
        └── Action: Write node condition node.gardener.cloud/CriticalComponentsReady=True
                    (instead of removing taint directly)

Shoot Cluster
└── node-readiness-controller (new deployment)
    └── NodeReadinessRule: critical-components-readiness
        ├── spec.conditions: [{type: "node.gardener.cloud/CriticalComponentsReady", requiredStatus: "True"}]
        ├── spec.taint.key: "readiness.k8s.io/gardener.cloud-critical-components-not-ready"
        └── spec.enforcementMode: bootstrap-only
```

### Phase 3 Target Architecture (Full Replacement)

```
Shoot Cluster Node (per node)
└── gardener-node-agent (extended)
    ├── Checks: DaemonSet scheduling, Pod readiness, CSINode drivers
    └── Writes: node condition node.gardener.cloud/CriticalComponentsReady=True/False

Shoot Cluster
└── node-readiness-controller
    └── NodeReadinessRule: critical-components-readiness
        ├── Watches: node condition node.gardener.cloud/CriticalComponentsReady
        └── Manages: taint readiness.k8s.io/gardener.cloud-critical-components-not-ready

Seed Cluster
└── gardener-resource-manager
    └── node-critical-components controller: REMOVED
```

---

## 4. Phased Implementation Plan

### Phase 1: Investigation & Experimental Deployment

**Goal:** Validate NRC works in a Gardener shoot cluster. Produce gap analysis. No production impact.

**Steps:**
1. Set up local Gardener environment:
   ```bash
   make kind-up
   make gardener-up
   ```
2. Deploy NRC into the shoot cluster manually (via `kubectl apply` of NRC manifests).
3. Create a `NodeReadinessRule` that mirrors Gardener's current behavior (using a dummy node condition).
4. Observe NRC behavior: taint application, removal, dry-run mode, status updates.
5. Document: what works, what doesn't, what needs upstream contributions.
6. Determine taint key migration strategy: either (a) file an upstream NRC issue to relax the `readiness.k8s.io/` prefix requirement, or (b) plan a Gardener-side migration from `node.gardener.cloud/critical-components-not-ready` to a new `readiness.k8s.io/`-prefixed key, auditing all tolerations in the codebase first.

**Deliverables:**
- This design document
- Experimental deployment notes
- Upstream contribution candidates (issues/PRs to `kubernetes-sigs/node-readiness-controller`)

**Risk:** None — no changes to existing Gardener code in this phase.

### Phase 2: Bridge — Resource-Manager Writes Conditions, NRC Manages Taints

**Goal:** Introduce NRC into Gardener's shoot cluster lifecycle. Resource-manager continues to do the heavy lifting (pod/CSI checks) but writes a node condition instead of removing the taint. NRC manages the taint.

**Code changes:**
1. **`pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go`**
   - Change the "all checks passed" action from `RemoveTaint()` to `WriteNodeCondition(node, "node.gardener.cloud/CriticalComponentsReady", True)`
   - Keep all existing pod/DaemonSet/CSI check logic unchanged

2. **NRC deployment as a shoot system component**
   - Add NRC as a managed component deployed to the shoot cluster (similar to node-problem-detector)
   - Location: `pkg/component/nodemanagement/nodereadinesscontroller/`
   - Gardener deploys NRC + a pre-configured `NodeReadinessRule` CR on every shoot

3. **Taint key migration**
   - New taint: `readiness.k8s.io/gardener.cloud-critical-components-not-ready:NoSchedule`
   - Old taint: `node.gardener.cloud/critical-components-not-ready:NoSchedule`
   - Transition: kubelet registers nodes with the new taint; resource-manager watches for either taint during migration window
   - Worker extension and gardenlet configuration updated accordingly

4. **Feature gate:** `NodeReadinessControllerIntegration` (alpha) — gates Phase 2 behavior; defaults off initially

**Files to change:**
- `pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go`
- `pkg/component/nodemanagement/nodereadinesscontroller/` (new)
- `pkg/gardenlet/operation/botanist/` (add NRC deployment)
- `pkg/component/extensions/operatingsystemconfig/original/components/kubelet/cliflags.go` (taint key)
- `charts/` or `pkg/component/` for NRC Helm chart/manifests
- `docs/concepts/resource-manager.md`
- `docs/usage/advanced/node-readiness.md`

**Testing:**
- Unit tests: condition-writing logic in reconciler
- Integration tests: NRC reacts to condition → taint removed
- Local: `make kind-up && make gardener-up`, create shoot, verify node bootstrap flow

**Risk:** Medium — taint key change affects kubelet config, worker extensions, tolerations across the codebase. Needs careful search of all taint key references.

### Phase 3: Full Replacement — Move Condition Writing to gardener-node-agent

**Goal:** Eliminate the resource-manager's node-critical-components controller entirely. gardener-node-agent runs on each node and writes the condition directly.

**Code changes:**
1. **`pkg/nodeagent/`** — Add new reconciler that:
   - Lists DaemonSets + Pods in kube-system with critical-component label
   - Checks pod readiness and CSI drivers
   - Writes node condition `node.gardener.cloud/CriticalComponentsReady`

2. **`pkg/resourcemanager/controller/node/criticalcomponents/`** — Delete entirely

3. **`pkg/resourcemanager/controller/node/add.go`** — Remove NodeCriticalComponentsController registration

4. **Feature gate promoted to beta/stable**

**Risk:** Higher — changes gardener-node-agent behavior on every node. Requires careful testing. Only proceed once Phase 2 is validated in production.

---

## 5. Upstream Contributions

The following gaps should be contributed upstream to `kubernetes-sigs/node-readiness-controller`:

| Gap | Proposed Contribution |
|-----|----------------------|
| Taint key prefix constraint (`readiness.k8s.io/` required) | Issue + PR to relax or make prefix configurable |
| No CSI driver awareness | Issue proposing a built-in CSI condition reporter sidecar |
| No pod-readiness condition reporter | Issue/PR for a DaemonSet-based pod-readiness reporter |
| Documentation for Gardener integration pattern | Blog post or docs PR to NRC repo |

---

## 6. Local Development Setup

Use the standard Gardener local setup:

```bash
# Prerequisites: Docker, Go, kubectl, kind, skaffold, helm, yq, jq
# Minimum resources: 8 CPUs, 8Gi memory

# 1. Create KinD cluster with local registry and DNS
make kind-up

# 2. Deploy all Gardener components via Skaffold
make gardener-up

# 3. Wait for seed readiness
./hack/usage/wait-for.sh seed local GardenletReady SeedSystemComponentsHealthy

# 4. Create a test shoot
kubectl apply -f example/provider-local/shoot.yaml

# For iterative development (watch mode):
make gardener-dev

# For debugging with Delve:
make gardener-debug
```

**Kubeconfig locations:**
- Garden/seed cluster: `example/gardener-local/kind/local/kubeconfig`
- Shoot cluster: created dynamically in `dev/local-kubeconfigs/` after shoot is Ready

---

## 7. Risk Assessment & Rollback

| Risk | Mitigation |
|------|-----------|
| NRC alpha instability | Feature gate; fallback to existing controller |
| Taint key migration breaks tolerations | Audit all taint references before Phase 2; dual-taint support during transition |
| gardener-node-agent pod-readiness checks are slower | Configurable check interval; Phase 3 only after Phase 2 validated |
| NRC not maintained upstream | Gardener can maintain a fork or revert to Phase 2 pattern |

**Rollback per phase:**
- Phase 1: No code changes — nothing to rollback
- Phase 2: Disable feature gate → resource-manager reverts to direct taint removal
- Phase 3: Re-enable resource-manager controller (it was deleted, so needs a revert commit)

---

## 8. Timing Recommendation

| Phase | Recommended Timeline | NRC Version Target |
|-------|---------------------|-------------------|
| Phase 1 (investigation) | Now (Q2 2026) | v0.1.1 (current) |
| Phase 2 (bridge) | Q3 2026 | v0.2+ (post-KubeCon EU 2026) |
| Phase 3 (full replacement) | Q4 2026 / Q1 2027 | v1.0 or beta |

Early adoption is recommended to establish Gardener's presence in the NRC project and influence its roadmap (particularly CSI awareness and taint key prefix flexibility).

---

## 9. Success Criteria

- [ ] Phase 1: NRC deployed in local Gardener shoot cluster; behavior documented; gaps filed upstream
- [ ] Phase 2: Shoots bootstrapped using NRC + resource-manager bridge, feature-gated; all existing tests pass
- [ ] Phase 3: `criticalcomponents` controller deleted; gardener-node-agent writes conditions; e2e tests cover full flow
- [ ] Upstream: At least one PR/issue filed to `kubernetes-sigs/node-readiness-controller`

---

## References

- [Gardener node critical components docs](https://github.com/gardener/gardener/blob/master/docs/concepts/resource-manager.md#critical-components-controller)
- [Gardener node readiness docs](https://github.com/gardener/gardener/blob/master/docs/usage/advanced/node-readiness.md)
- [Original issue #7117](https://github.com/gardener/gardener/issues/7117)
- [NRC GitHub repo](https://github.com/kubernetes-sigs/node-readiness-controller)
- [NRC Kubernetes blog post](https://kubernetes.io/blog/2026/02/03/introducing-node-readiness-controller/)
- [KEP-5233: NodeReadinessGates](https://github.com/kubernetes/enhancements/pull/5416)
