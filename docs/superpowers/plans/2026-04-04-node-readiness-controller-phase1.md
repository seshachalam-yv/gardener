# Node Readiness Controller — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the upstream Node Readiness Controller (NRC) experimentally into a local Gardener shoot cluster, validate its behavior, document gaps, and wire it up as a proper Gardener shoot system component (via ManagedResource) behind a feature gate — all validated with `make kind-up && make gardener-up` and the local provider-local shoot.

**Architecture:** NRC runs as a `Deployment` in the shoot cluster's `kube-system` namespace, deployed via Gardener's standard ManagedResource pattern (identical to node-problem-detector). The resource-manager's existing `node-critical-components` controller is modified to write a node condition (`node.gardener.cloud/CriticalComponentsReady`) instead of removing the taint directly. NRC watches that condition via a `NodeReadinessRule` CR and manages the new taint `readiness.k8s.io/gardener-critical-components-not-ready`. A new feature gate `NodeReadinessController` (alpha, default false) gates all new behavior.

**Tech Stack:** Go 1.25.6, controller-runtime, Gardener ManagedResource pattern, `make kind-up` / `make gardener-up` / `make gardener-dev` for local validation, Ginkgo/Gomega for tests.

---

## File Map

| Action | Path | Responsibility |
|--------|------|---------------|
| Create | `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller.go` | NRC component: builds and deploys ManagedResource containing NRC Deployment + CRD + RBAC + NodeReadinessRule CR |
| Create | `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_test.go` | Unit tests for the component |
| Create | `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_suite_test.go` | Ginkgo suite bootstrap |
| Modify | `pkg/features/features.go` | Add `NodeReadinessController` alpha feature gate |
| Modify | `pkg/apis/core/v1beta1/constants/types_constants.go` | Add new taint key constant + new condition type constant |
| Modify | `pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go` | When feature gate enabled: write node condition instead of removing taint |
| Modify | `pkg/resourcemanager/controller/node/criticalcomponents/add.go` | Watch both old and new taint keys when feature gate enabled |
| Modify | `pkg/component/extensions/operatingsystemconfig/original/components/kubelet/config.go` | When feature gate enabled: register node with new taint key |
| Modify | `pkg/gardenlet/operation/botanist/nodeproblemdetector.go` | Add `DefaultNodeReadinessController()` function (new file actually) |
| Create | `pkg/gardenlet/operation/botanist/nodereadinesscontroller.go` | Botanist wiring for NRC component |
| Modify | `pkg/gardenlet/operation/botanist/botanist.go` OR shoot reconcile flow | Call `DeployNodeReadinessController` during shoot reconciliation |
| Create | `test/integration/resourcemanager/node/criticalcomponents/nodereadiness_test.go` | Integration test: condition written correctly |

---

## Task 1: Add constants and feature gate

**Files:**
- Modify: `pkg/apis/core/v1beta1/constants/types_constants.go`
- Modify: `pkg/features/features.go`

- [ ] **Step 1.1: Read the constants file to find insertion point**

```bash
grep -n "TaintNodeCriticalComponentsNotReady\|LabelNodeCriticalComponent\|AnnotationPrefixWaitForCSINode" \
  pkg/apis/core/v1beta1/constants/types_constants.go
```

Expected output: lines around 1077 showing the existing constants.

- [ ] **Step 1.2: Add new constants to `pkg/apis/core/v1beta1/constants/types_constants.go`**

Find the block containing `TaintNodeCriticalComponentsNotReady = "node.gardener.cloud/critical-components-not-ready"` and add immediately after it:

```go
	// TaintNodeReadinessControllerNotReady is the taint key managed by the upstream Node Readiness Controller
	// when the NodeReadinessController feature gate is enabled. Uses the readiness.k8s.io/ prefix required by NRC.
	TaintNodeReadinessControllerNotReady = "readiness.k8s.io/gardener-critical-components-not-ready"
	// NodeConditionCriticalComponentsReady is the node condition type written by the resource-manager's
	// node-critical-components controller when the NodeReadinessController feature gate is enabled.
	// The upstream NRC reacts to this condition to manage the taint.
	NodeConditionCriticalComponentsReady = "node.gardener.cloud/CriticalComponentsReady"
```

- [ ] **Step 1.3: Add feature gate to `pkg/features/features.go`**

In the `const` block, add after the last existing feature gate (before the closing `)`):

```go
	// NodeReadinessController enables integration with the upstream Kubernetes Node Readiness Controller
	// (github.com/kubernetes-sigs/node-readiness-controller) as a replacement for Gardener's
	// node-critical-components controller. When enabled, the resource-manager writes a node condition
	// instead of removing the taint directly, and NRC is deployed to the shoot cluster to manage taints.
	// owner: @seshachalam-yv
	// alpha: v1.141.0
	NodeReadinessController featuregate.Feature = "NodeReadinessController"
```

In `AllFeatureGates`, add:

```go
	NodeReadinessController: {Default: false, PreRelease: featuregate.Alpha},
```

- [ ] **Step 1.4: Verify the project builds**

```bash
go build ./pkg/features/... ./pkg/apis/core/v1beta1/constants/...
```

Expected: no output (success).

- [ ] **Step 1.5: Commit**

```bash
git add pkg/apis/core/v1beta1/constants/types_constants.go pkg/features/features.go
git commit -m "feat(node): add NodeReadinessController feature gate and new taint/condition constants"
```

---

## Task 2: Modify the resource-manager reconciler to write a node condition

**Files:**
- Modify: `pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go`

- [ ] **Step 2.1: Read the reconciler to understand the current Reconcile function**

```bash
grep -n "RemoveTaint\|func.*Reconcile\|log.Info.*ready" \
  pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go
```

Expected: lines showing `RemoveTaint` call at ~line 99 and the success log at ~line 97.

- [ ] **Step 2.2: Add `WriteNodeCondition` function and modify `Reconcile` in `pkg/resourcemanager/controller/node/criticalcomponents/reconciler.go`**

Add the following import to the existing import block (the file already imports `corev1`, `metav1`, `client`):

```go
	"time"

	"github.com/gardener/gardener/pkg/features"
	"k8s.io/apimachinery/pkg/api/equality"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
```

Replace the success block at the end of `Reconcile` (the lines after all three checks pass):

```go
	log.Info("All node-critical components got ready, removing taint")
	r.Recorder.Eventf(node, nil, corev1.EventTypeNormal, "NodeCriticalComponentsReady", gardencorev1beta1.EventActionReconcile, "All node-critical components got ready, removing taint")
	return reconcile.Result{}, RemoveTaint(ctx, r.TargetClient, node)
```

Replace with:

```go
	log.Info("All node-critical components got ready")
	r.Recorder.Eventf(node, nil, corev1.EventTypeNormal, "NodeCriticalComponentsReady", gardencorev1beta1.EventActionReconcile, "All node-critical components got ready")

	if utilfeature.DefaultFeatureGate.Enabled(features.NodeReadinessController) {
		// When NRC integration is enabled, write a node condition that the upstream NRC will react to.
		// NRC manages the taint; we no longer remove it directly.
		return reconcile.Result{}, WriteNodeConditionCriticalComponentsReady(ctx, r.TargetClient, node)
	}

	log.Info("Removing taint (legacy mode)")
	return reconcile.Result{}, RemoveTaint(ctx, r.TargetClient, node)
```

Add `WriteNodeConditionCriticalComponentsReady` function at the bottom of the file:

```go
// WriteNodeConditionCriticalComponentsReady patches the node's status to set the
// NodeConditionCriticalComponentsReady condition to True. This is used when the
// NodeReadinessController feature gate is enabled — the upstream NRC reacts to
// this condition and removes the taint instead of this controller doing it directly.
func WriteNodeConditionCriticalComponentsReady(ctx context.Context, c client.Client, node *corev1.Node) error {
	patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})

	now := metav1.NewTime(time.Now())
	newCondition := corev1.NodeCondition{
		Type:               corev1.NodeConditionType(v1beta1constants.NodeConditionCriticalComponentsReady),
		Status:             corev1.ConditionTrue,
		Reason:             "AllCriticalComponentsReady",
		Message:            "All node-critical DaemonSet pods are scheduled and ready, and all required CSI drivers are registered.",
		LastTransitionTime: now,
		LastHeartbeatTime:  now,
	}

	// Check if condition already exists with the same status to avoid unnecessary patches.
	for i, c := range node.Status.Conditions {
		if c.Type == newCondition.Type {
			if c.Status == newCondition.Status {
				return nil
			}
			// Update existing condition.
			node.Status.Conditions[i] = newCondition
			node.Status.Conditions[i].LastTransitionTime = c.LastTransitionTime // preserve original transition time
			return c.Patch(ctx, node, patch)
		}
	}

	// Condition not found — append it.
	node.Status.Conditions = append(node.Status.Conditions, newCondition)
	return c.Patch(ctx, node, patch)
}
```

**Note:** `node.Status.Conditions` is a status subresource; we use `StatusClient` for the patch. Fix the last two `c.Patch` calls:

```go
// Replace: return c.Patch(ctx, node, patch)  (inside the loop)
// With:
			return c.Status().Patch(ctx, node, patch)
// And:
// Replace: return c.Patch(ctx, node, patch)  (after append)
// With:
	return c.Status().Patch(ctx, node, patch)
```

The full corrected `WriteNodeConditionCriticalComponentsReady`:

```go
func WriteNodeConditionCriticalComponentsReady(ctx context.Context, w client.Client, node *corev1.Node) error {
	patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})

	now := metav1.NewTime(time.Now())
	newCondition := corev1.NodeCondition{
		Type:              corev1.NodeConditionType(v1beta1constants.NodeConditionCriticalComponentsReady),
		Status:            corev1.ConditionTrue,
		Reason:            "AllCriticalComponentsReady",
		Message:           "All node-critical DaemonSet pods are scheduled and ready, and all required CSI drivers are registered.",
		LastHeartbeatTime: now,
	}

	for i, existing := range node.Status.Conditions {
		if existing.Type == newCondition.Type {
			if existing.Status == newCondition.Status {
				return nil // already set, nothing to do
			}
			newCondition.LastTransitionTime = existing.LastTransitionTime // preserve
			node.Status.Conditions[i] = newCondition
			return w.Status().Patch(ctx, node, patch)
		}
	}

	newCondition.LastTransitionTime = now
	node.Status.Conditions = append(node.Status.Conditions, newCondition)
	return w.Status().Patch(ctx, node, patch)
}
```

- [ ] **Step 2.3: Write the failing unit test first**

Create file `pkg/resourcemanager/controller/node/criticalcomponents/reconciler_nodereadiness_test.go`:

```go
// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package criticalcomponents_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	. "github.com/gardener/gardener/pkg/resourcemanager/controller/node/criticalcomponents"
	"github.com/gardener/gardener/pkg/features"
	featureutils "k8s.io/component-base/featuregate/testing"
)

var _ = Describe("WriteNodeConditionCriticalComponentsReady", func() {
	var (
		node   *corev1.Node
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = kubernetes.SeedScheme
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		}
	})

	It("should set the CriticalComponentsReady condition to True on a node with no prior conditions", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&corev1.Node{}).WithObjects(node).Build()

		Expect(WriteNodeConditionCriticalComponentsReady(ctx, fakeClient, node)).To(Succeed())

		updated := &corev1.Node{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(node), updated)).To(Succeed())

		cond := findCondition(updated.Status.Conditions, corev1.NodeConditionType(constants.NodeConditionCriticalComponentsReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(corev1.ConditionTrue))
		Expect(cond.Reason).To(Equal("AllCriticalComponentsReady"))
	})

	It("should be a no-op if condition is already True", func() {
		node.Status.Conditions = []corev1.NodeCondition{{
			Type:   corev1.NodeConditionType(constants.NodeConditionCriticalComponentsReady),
			Status: corev1.ConditionTrue,
		}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&corev1.Node{}).WithObjects(node).Build()

		Expect(WriteNodeConditionCriticalComponentsReady(ctx, fakeClient, node)).To(Succeed())
		// fake client patch count can be verified if needed
	})
})

func findCondition(conditions []corev1.NodeCondition, t corev1.NodeConditionType) *corev1.NodeCondition {
	for i := range conditions {
		if conditions[i].Type == t {
			return &conditions[i]
		}
	}
	return nil
}
```

- [ ] **Step 2.4: Run the test to verify it fails (function not yet exported/present)**

```bash
cd /Users/I568019/go/src/github.com/gardener/gardener
go test ./pkg/resourcemanager/controller/node/criticalcomponents/... -run "WriteNodeCondition" -v 2>&1 | tail -20
```

Expected: compile error or FAIL — `WriteNodeConditionCriticalComponentsReady` not found yet.

- [ ] **Step 2.5: Implement `WriteNodeConditionCriticalComponentsReady` as described in Step 2.2**

- [ ] **Step 2.6: Run the test to verify it passes**

```bash
go test ./pkg/resourcemanager/controller/node/criticalcomponents/... -run "WriteNodeCondition" -v 2>&1 | tail -20
```

Expected: `PASS`

- [ ] **Step 2.7: Run all existing critical-components tests to verify nothing broke**

```bash
go test ./pkg/resourcemanager/controller/node/criticalcomponents/... -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 2.8: Commit**

```bash
git add pkg/resourcemanager/controller/node/criticalcomponents/
git commit -m "feat(node): write node condition when NodeReadinessController feature gate is enabled"
```

---

## Task 3: Modify kubelet config to register with new taint when feature gate is enabled

**Files:**
- Modify: `pkg/component/extensions/operatingsystemconfig/original/components/kubelet/config.go`

- [ ] **Step 3.1: Read the current taint registration block**

```bash
sed -n '25,45p' pkg/component/extensions/operatingsystemconfig/original/components/kubelet/config.go
```

Expected output showing:
```go
nodeTaints := append(taints, corev1.Taint{
    Key:    v1beta1constants.TaintNodeCriticalComponentsNotReady,
    Effect: corev1.TaintEffectNoSchedule,
})
```

- [ ] **Step 3.2: Modify `config.go` to conditionally use new taint key**

Add import to the file's import block:
```go
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"github.com/gardener/gardener/pkg/features"
```

Replace the existing taint block:
```go
	nodeTaints := append(taints, corev1.Taint{
		Key:    v1beta1constants.TaintNodeCriticalComponentsNotReady,
		Effect: corev1.TaintEffectNoSchedule,
	})
```

With:
```go
	criticalComponentsTaintKey := v1beta1constants.TaintNodeCriticalComponentsNotReady
	if utilfeature.DefaultFeatureGate.Enabled(features.NodeReadinessController) {
		criticalComponentsTaintKey = v1beta1constants.TaintNodeReadinessControllerNotReady
	}
	nodeTaints := append(taints, corev1.Taint{
		Key:    criticalComponentsTaintKey,
		Effect: corev1.TaintEffectNoSchedule,
	})
```

- [ ] **Step 3.3: Write the failing test**

Find the kubelet config tests:
```bash
ls pkg/component/extensions/operatingsystemconfig/original/components/kubelet/
```

Look for `config_test.go`. Add a test case for the new taint key. In the existing test file that tests `Config(...)`, add a new `It` block:

```go
It("should use the new NRC taint key when NodeReadinessController feature gate is enabled", func() {
    featureutils.SetFeatureGateDuringTest(GinkgoT(), features.DefaultFeatureGate, features.NodeReadinessController, true)

    cfg := Config(semver.MustParse("1.30.0"), []string{"10.0.0.10"}, "cluster.local", nil, components.ConfigurableKubeletConfigParameters{})

    Expect(cfg.RegisterWithTaints).To(ContainElement(corev1.Taint{
        Key:    v1beta1constants.TaintNodeReadinessControllerNotReady,
        Effect: corev1.TaintEffectNoSchedule,
    }))
    Expect(cfg.RegisterWithTaints).NotTo(ContainElement(corev1.Taint{
        Key:    v1beta1constants.TaintNodeCriticalComponentsNotReady,
        Effect: corev1.TaintEffectNoSchedule,
    }))
})
```

- [ ] **Step 3.4: Run test to verify it fails**

```bash
go test ./pkg/component/extensions/operatingsystemconfig/original/components/kubelet/... -run "new NRC taint" -v 2>&1 | tail -20
```

Expected: FAIL — old taint key found in result.

- [ ] **Step 3.5: Implement the change from Step 3.2**

- [ ] **Step 3.6: Run test to verify it passes**

```bash
go test ./pkg/component/extensions/operatingsystemconfig/original/components/kubelet/... -run "new NRC taint" -v 2>&1 | tail -20
```

Expected: PASS

- [ ] **Step 3.7: Run full kubelet config tests**

```bash
go test ./pkg/component/extensions/operatingsystemconfig/original/components/kubelet/... -v 2>&1 | tail -30
```

Expected: all pass.

- [ ] **Step 3.8: Commit**

```bash
git add pkg/component/extensions/operatingsystemconfig/original/components/kubelet/config.go \
        pkg/component/extensions/operatingsystemconfig/original/components/kubelet/config_test.go
git commit -m "feat(kubelet): use NRC taint key when NodeReadinessController feature gate is enabled"
```

---

## Task 4: Create the NRC shoot system component

**Files:**
- Create: `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller.go`
- Create: `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_suite_test.go`
- Create: `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_test.go`

This component deploys NRC into the shoot cluster via ManagedResource, following the exact same pattern as `nodeproblemdetector`.

- [ ] **Step 4.1: Write the failing test first**

Create `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_suite_test.go`:

```go
// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodereadinesscontroller_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var ctx = context.TODO()

func TestNodeReadinessController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeReadinessController Suite")
}
```

Create `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller_test.go`:

```go
// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodereadinesscontroller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	. "github.com/gardener/gardener/pkg/component/nodemanagement/nodereadinesscontroller"
)

var _ = Describe("NodeReadinessController", func() {
	const (
		namespace = "shoot--foo--bar"
		image     = "registry.k8s.io/node-readiness-controller:v0.3.0"
	)

	var (
		fakeClient   *fake.ClientBuilder
		scheme       *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = kubernetes.SeedScheme
		fakeClient = fake.NewClientBuilder().WithScheme(scheme)
	})

	Describe("#Deploy", func() {
		It("should create a ManagedResource for the shoot", func() {
			c := fakeClient.Build()
			component := New(c, namespace, Values{Image: image})

			Expect(component.Deploy(ctx)).To(Succeed())

			mr := &resourcesv1alpha1.ManagedResource{}
			Expect(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ManagedResourceName}, mr)).To(Succeed())
			Expect(mr.Labels).To(HaveKeyWithValue("origin", "gardener"))
		})
	})

	Describe("#Destroy", func() {
		It("should delete the ManagedResource", func() {
			c := fakeClient.Build()
			component := New(c, namespace, Values{Image: image})

			Expect(component.Deploy(ctx)).To(Succeed())
			Expect(component.Destroy(ctx)).To(Succeed())

			mr := &resourcesv1alpha1.ManagedResource{}
			Expect(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ManagedResourceName}, mr)).To(MatchError(ContainSubstring("not found")))
		})
	})
})
```

- [ ] **Step 4.2: Run the test to verify it fails (package not yet created)**

```bash
go test ./pkg/component/nodemanagement/nodereadinesscontroller/... -v 2>&1 | tail -10
```

Expected: compile error — package not found.

- [ ] **Step 4.3: Create the component implementation**

Create `pkg/component/nodemanagement/nodereadinesscontroller/node_readiness_controller.go`:

```go
// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodereadinesscontroller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/managedresources"
)

const (
	// ManagedResourceName is the name of the ManagedResource containing the NRC manifests for the shoot.
	ManagedResourceName = "shoot-core-node-readiness-controller"

	labelValue             = "node-readiness-controller"
	serviceAccountName     = "node-readiness-controller"
	deploymentName         = "node-readiness-controller"
	clusterRoleName        = "node-readiness-controller"
	clusterRoleBindingName = "node-readiness-controller"

	// nrcTaintKey is the taint managed by NRC for Gardener's critical components gate.
	// Must use the readiness.k8s.io/ prefix required by NRC.
	nrcTaintKey = v1beta1constants.TaintNodeReadinessControllerNotReady

	// nodeReadinessRuleName is the name of the NodeReadinessRule CR deployed to the shoot.
	nodeReadinessRuleName = "gardener-critical-components"

	// nodeConditionType is the node condition NRC watches.
	nodeConditionType = v1beta1constants.NodeConditionCriticalComponentsReady
)

// Values is a set of configuration values for the NRC component.
type Values struct {
	// Image is the container image for the NRC deployment.
	Image string
}

// New creates a new instance of the DeployWaiter for the Node Readiness Controller.
func New(client client.Client, namespace string, values Values) component.DeployWaiter {
	return &nodeReadinessController{
		client:    client,
		namespace: namespace,
		values:    values,
	}
}

type nodeReadinessController struct {
	client    client.Client
	namespace string
	values    Values
}

func (n *nodeReadinessController) Deploy(ctx context.Context) error {
	data, err := n.computeResourcesData()
	if err != nil {
		return err
	}
	return managedresources.CreateForShoot(ctx, n.client, n.namespace, ManagedResourceName, managedresources.LabelValueGardener, false, data)
}

func (n *nodeReadinessController) Destroy(ctx context.Context) error {
	return managedresources.DeleteForShoot(ctx, n.client, n.namespace, ManagedResourceName)
}

// TimeoutWaitForManagedResource is the timeout used while waiting for the ManagedResource to become healthy.
var TimeoutWaitForManagedResource = 2 * time.Minute

func (n *nodeReadinessController) Wait(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForManagedResource)
	defer cancel()
	return managedresources.WaitUntilHealthy(timeoutCtx, n.client, n.namespace, ManagedResourceName)
}

func (n *nodeReadinessController) WaitCleanup(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForManagedResource)
	defer cancel()
	return managedresources.WaitUntilDeleted(timeoutCtx, n.client, n.namespace, ManagedResourceName)
}

func (n *nodeReadinessController) computeResourcesData() (map[string][]byte, error) {
	registry := managedresources.NewRegistry(kubernetes.ShootScheme, kubernetes.ShootCodec, kubernetes.ShootSerializer)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: metav1.NamespaceSystem,
			Labels:    getLabels(),
		},
		AutomountServiceAccountToken: ptr.To(false),
	}

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleName,
			Labels: getLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "watch", "patch", "update"},
			},
			{
				APIGroups: []string{"readiness.node.x-k8s.io"},
				Resources: []string{"nodereadinessrules"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"readiness.node.x-k8s.io"},
				Resources: []string{"nodereadinessrules/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
		},
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleBindingName,
			Labels: getLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      serviceAccount.Name,
			Namespace: serviceAccount.Namespace,
		}},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: metav1.NamespaceSystem,
			Labels: utils.MergeStringMaps(getLabels(), map[string]string{
				managedresources.LabelKeyOrigin: managedresources.LabelValueGardener,
				v1beta1constants.GardenRole:     v1beta1constants.GardenRoleSystemComponent,
			}),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.MergeStringMaps(getLabels(), map[string]string{
						v1beta1constants.GardenRole:     v1beta1constants.GardenRoleSystemComponent,
						managedresources.LabelKeyOrigin: managedresources.LabelValueGardener,
					}),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount.Name,
					PriorityClassName:  v1beta1constants.PriorityClassNameShootSystem900,
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Tolerations: []corev1.Toleration{
						// Must tolerate both taint keys during migration
						{
							Key:      v1beta1constants.TaintNodeCriticalComponentsNotReady,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
						{
							Key:      nrcTaintKey,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "node-readiness-controller",
							Image:           n.values.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
							},
						},
					},
				},
			},
		},
	}

	// NodeReadinessRule CR — tells NRC to watch our condition and manage our taint.
	// This is an unstructured object since we don't vendor the NRC CRD types.
	nodeReadinessRule := nodeReadinessRuleObject()

	return registry.AddAllAndSerialize(
		serviceAccount,
		clusterRole,
		clusterRoleBinding,
		deployment,
		nodeReadinessRule,
	)
}

// nodeReadinessRuleObject returns the NodeReadinessRule CR as an unstructured object.
// We use unstructured because the NRC types are not vendored in Gardener.
func nodeReadinessRuleObject() *runtime.Unknown {
	// We use AddSerialized to add raw YAML since the NRC CRD types are not part of
	// kubernetes.ShootScheme. The registry will include this verbatim in the ManagedResource secret.
	return nil // placeholder — see Step 4.4 for the actual implementation
}

func getLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     labelValue,
		"app.kubernetes.io/instance": "shoot-core",
	}
}
```

**Note:** The `nodeReadinessRuleObject` approach using `runtime.Unknown` won't work with `AddAllAndSerialize`. Instead use `AddSerialized` for the raw YAML. See Step 4.4 for the corrected implementation.

- [ ] **Step 4.4: Fix the NodeReadinessRule serialization using `AddSerialized`**

Replace `nodeReadinessRuleObject()` call in `computeResourcesData` with a direct `registry.AddSerialized` call. The full corrected `computeResourcesData` ending:

```go
	if err := registry.Add(serviceAccount, clusterRole, clusterRoleBinding, deployment); err != nil {
		return nil, err
	}

	// NodeReadinessRule CR is added as raw YAML since the NRC CRD types are not vendored.
	registry.AddSerialized("nodereadinessrule__gardener-critical-components.yaml", nodeReadinessRuleYAML())

	return registry.SerializedObjects()
```

Add helper:

```go
func nodeReadinessRuleYAML() []byte {
	return []byte(`apiVersion: readiness.node.x-k8s.io/v1alpha1
kind: NodeReadinessRule
metadata:
  name: ` + nodeReadinessRuleName + `
spec:
  conditions:
    - type: "` + nodeConditionType + `"
      requiredStatus: "True"
  taint:
    key: "` + nrcTaintKey + `"
    effect: "NoSchedule"
    value: "pending"
  enforcementMode: "bootstrap-only"
  nodeSelector:
    matchLabels: {}
`)
}
```

**Remove** the `nodeReadinessRuleObject` function and `runtime.Unknown` usage.

Also remove `"k8s.io/apimachinery/pkg/runtime/serializer"` and `apiextensionsv1` from imports if unused.

- [ ] **Step 4.5: Run the test to verify it passes**

```bash
go test ./pkg/component/nodemanagement/nodereadinesscontroller/... -v 2>&1 | tail -20
```

Expected: PASS

- [ ] **Step 4.6: Verify the build**

```bash
go build ./pkg/component/nodemanagement/nodereadinesscontroller/...
```

Expected: no output.

- [ ] **Step 4.7: Commit**

```bash
git add pkg/component/nodemanagement/nodereadinesscontroller/
git commit -m "feat(node): add NRC shoot system component (ManagedResource deployment)"
```

---

## Task 5: Wire NRC into the gardenlet botanist

**Files:**
- Create: `pkg/gardenlet/operation/botanist/nodereadinesscontroller.go`
- Modify: shoot reconciliation flow to call NRC deploy/destroy

- [ ] **Step 5.1: Find where NPD is deployed in the shoot reconciliation flow**

```bash
grep -rn "DefaultNodeProblemDetector\|DeployNodeProblemDetector\|NodeProblemDetector" \
  pkg/gardenlet/operation/botanist/ pkg/gardenlet/controller/shoot/ 2>/dev/null | head -20
```

Note the file and function name where NPD is called — this is where NRC should be added.

- [ ] **Step 5.2: Create `pkg/gardenlet/operation/botanist/nodereadinesscontroller.go`**

```go
// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package botanist

import (
	"github.com/gardener/gardener/imagevector"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/component/nodemanagement/nodereadinesscontroller"
	"github.com/gardener/gardener/pkg/features"
	imagevectorutils "github.com/gardener/gardener/pkg/utils/imagevector"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

// DefaultNodeReadinessController returns a deployer for the Node Readiness Controller.
// Returns nil if the NodeReadinessController feature gate is disabled.
func (b *Botanist) DefaultNodeReadinessController() (component.DeployWaiter, error) {
	if !utilfeature.DefaultFeatureGate.Enabled(features.NodeReadinessController) {
		return component.NoOp(), nil
	}

	image, err := imagevector.Containers().FindImage(
		imagevector.ContainerImageNameNodeReadinessController,
		imagevectorutils.RuntimeVersion(b.ShootVersion()),
		imagevectorutils.TargetVersion(b.ShootVersion()),
	)
	if err != nil {
		return nil, err
	}

	return nodereadinesscontroller.New(
		b.SeedClientSet.Client(),
		b.Shoot.ControlPlaneNamespace,
		nodereadinesscontroller.Values{Image: image.String()},
	), nil
}
```

- [ ] **Step 5.3: Add the NRC image to the image vector**

Find the image vector file:
```bash
grep -n "node-problem-detector\|NodeProblemDetector" imagevector/containers.yaml | head -5
```

Note the format, then add the NRC image entry to `imagevector/containers.yaml` (or the equivalent `imagevector/images.go` / `imagevector/containers.go`):

```yaml
- name: node-readiness-controller
  sourceRepository: github.com/kubernetes-sigs/node-readiness-controller
  repository: registry.k8s.io/node-readiness-controller/controller
  tag: "v0.3.0"
```

Also add the constant `ContainerImageNameNodeReadinessController` to the image vector constants file. Find it:
```bash
grep -n "ContainerImageNameNodeProblemDetector" imagevector/
```

Add alongside it:
```go
// ContainerImageNameNodeReadinessController is the name of the node-readiness-controller container image.
ContainerImageNameNodeReadinessController = "node-readiness-controller"
```

- [ ] **Step 5.4: Wire DefaultNodeReadinessController into shoot reconciliation**

Find the exact location in the shoot reconciler where `DefaultNodeProblemDetector` is called and add NRC alongside it:

```bash
grep -rn "DefaultNodeProblemDetector\|b\.Shoot\.Components\.SystemComponents" \
  pkg/gardenlet/ | head -20
```

In the same place (typically `pkg/gardenlet/operation/botanist/initializeinterfaces.go` or similar), add:

```go
b.Shoot.Components.SystemComponents.NodeReadinessController, err = b.DefaultNodeReadinessController()
if err != nil {
    return err
}
```

Also find the `Shoot.Components.SystemComponents` struct definition and add the field:

```bash
grep -rn "NodeProblemDetector.*component\." pkg/gardenlet/operation/ | head -5
```

Add `NodeReadinessController component.DeployWaiter` to the struct.

- [ ] **Step 5.5: Add NRC to the deploy/destroy reconciliation steps**

Find where `NodeProblemDetector.Deploy()` and `NodeProblemDetector.Destroy()` are called:

```bash
grep -rn "NodeProblemDetector\.Deploy\|NodeProblemDetector\.Destroy" pkg/gardenlet/ | head -10
```

Add NRC calls in the same places:
```go
// In deploy:
if err := b.Shoot.Components.SystemComponents.NodeReadinessController.Deploy(ctx); err != nil {
    return err
}
// In destroy:
if err := b.Shoot.Components.SystemComponents.NodeReadinessController.Destroy(ctx); err != nil {
    return err
}
```

- [ ] **Step 5.6: Build to check for compile errors**

```bash
go build ./pkg/gardenlet/...
```

Fix any compile errors.

- [ ] **Step 5.7: Run gardenlet tests**

```bash
go test ./pkg/gardenlet/... 2>&1 | tail -30
```

Expected: all existing tests pass.

- [ ] **Step 5.8: Commit**

```bash
git add pkg/gardenlet/operation/botanist/nodereadinesscontroller.go \
        imagevector/containers.yaml \
        pkg/gardenlet/
git commit -m "feat(gardenlet): wire NRC into shoot system components deployment"
```

---

## Task 6: Local validation with kind

**Prerequisite:** Docker running, 8 CPUs + 8Gi RAM available.

- [ ] **Step 6.1: Stand up the local Gardener environment**

```bash
cd /Users/I568019/go/src/github.com/gardener/gardener
make kind-up
```

Expected: KinD cluster `gardener-local` created, local registry running at `localhost:5001`.

- [ ] **Step 6.2: Deploy Gardener components**

```bash
make gardener-up
```

Expected: skaffold deploys all components. Wait for completion (~3-5 min).

- [ ] **Step 6.3: Wait for seed to be ready**

```bash
KUBECONFIG=example/gardener-local/kind/local/kubeconfig \
  ./hack/usage/wait-for.sh seed local GardenletReady SeedSystemComponentsHealthy
```

Expected: both conditions become True.

- [ ] **Step 6.4: Enable the feature gate in gardenlet config and redeploy**

Find the gardenlet configuration (likely in `example/gardener-local/` or generated by skaffold):

```bash
grep -rn "featureGates\|FeatureGates" example/gardener-local/ charts/gardener/gardenlet/ | head -10
```

Add the feature gate to the gardenlet's featureGates section:
```yaml
featureGates:
  NodeReadinessController: true
```

Redeploy:
```bash
make gardener-dev
# or: skaffold run -m gardenlet
```

- [ ] **Step 6.5: Create a test shoot**

```bash
KUBECONFIG=example/gardener-local/kind/local/kubeconfig \
  kubectl apply -f example/provider-local/shoot.yaml
```

Wait for shoot to become Ready:
```bash
KUBECONFIG=example/gardener-local/kind/local/kubeconfig \
  kubectl get shoot local -n garden-local -w
```

Expected: `STATUS = Healthy` eventually.

- [ ] **Step 6.6: Verify NRC is deployed in the shoot cluster**

Get shoot kubeconfig (path printed by gardenlet or in `dev/local-kubeconfigs/`):
```bash
KUBECONFIG=<shoot-kubeconfig> kubectl get deployment -n kube-system node-readiness-controller
```

Expected: `node-readiness-controller` deployment running with 1/1 ready.

- [ ] **Step 6.7: Verify the NodeReadinessRule CR exists**

```bash
KUBECONFIG=<shoot-kubeconfig> kubectl get nodereadinessrule gardener-critical-components -o yaml
```

Expected: CR present with `spec.taint.key: readiness.k8s.io/gardener-critical-components-not-ready`.

- [ ] **Step 6.8: Verify node bootstrap flow**

```bash
KUBECONFIG=<shoot-kubeconfig> kubectl get nodes -o custom-columns=\
NAME:.metadata.name,TAINTS:.spec.taints,CONDITIONS:.status.conditions
```

Expected for a Ready node: no `readiness.k8s.io/gardener-critical-components-not-ready` taint, and `node.gardener.cloud/CriticalComponentsReady=True` condition present.

- [ ] **Step 6.9: Verify the NodeReadinessRule status shows the node as applied**

```bash
KUBECONFIG=<shoot-kubeconfig> kubectl get nodereadinessrule gardener-critical-components -o jsonpath='{.status.appliedNodes}'
```

Expected: list containing the node name.

- [ ] **Step 6.10: Tear down**

```bash
make gardener-down
make kind-down
```

- [ ] **Step 6.11: Commit validation notes**

```bash
git add docs/superpowers/
git commit -m "docs: add local validation notes for NRC phase 1"
```

---

## Task 7: Self-review and spec coverage check

- [ ] **Step 7.1: Verify all spec success criteria are met**

From the design spec section 9:
- [ ] Phase 1: NRC deployed in local Gardener shoot cluster ✓ (Task 6)
- [ ] behavior documented ✓ (Task 6 steps 6.6–6.9)
- [ ] gaps filed upstream — file an issue at https://github.com/kubernetes-sigs/node-readiness-controller/issues describing the taint key prefix constraint and lack of CSI awareness

- [ ] **Step 7.2: File upstream issues**

File two issues on `kubernetes-sigs/node-readiness-controller`:
1. "Support configurable taint key prefix (not just readiness.k8s.io/)" — describe Gardener's use case
2. "Add CSI driver awareness via built-in condition reporter" — describe the `CSINode.Spec.Drivers` check Gardener currently does

- [ ] **Step 7.3: Run the full unit test suite**

```bash
go test \
  ./pkg/features/... \
  ./pkg/apis/core/v1beta1/constants/... \
  ./pkg/resourcemanager/controller/node/... \
  ./pkg/component/nodemanagement/nodereadinesscontroller/... \
  ./pkg/component/extensions/operatingsystemconfig/original/components/kubelet/... \
  2>&1 | tail -30
```

Expected: all PASS.

- [ ] **Step 7.4: Final commit and branch push**

```bash
git log --oneline -10
git push origin HEAD
```

---

## Self-Review Notes

**Spec coverage:**
- ✅ Phase 1 investigation: Tasks 1–6 cover experimental deployment + gap analysis
- ✅ Feature gate: Task 1 adds `NodeReadinessController` alpha gate
- ✅ NRC deployed as ManagedResource: Task 4
- ✅ Condition writing in resource-manager: Task 2
- ✅ Taint key migration: Task 3 (kubelet uses new key when gate enabled)
- ✅ Local kind validation: Task 6
- ⚠️ NRC CRD installation in shoot: The NodeReadinessRule CRD must be installed before the NodeReadinessRule CR. Add CRD YAML to the ManagedResource in Task 4 (via `AddSerialized` for the CRD YAML from upstream). Fetch the CRD from: `https://raw.githubusercontent.com/kubernetes-sigs/node-readiness-controller/main/config/crd/bases/readiness.node.x-k8s.io_nodereadinessrules.yaml`

**Additional step for Task 4 (CRD):** In `computeResourcesData`, after adding the objects, also add:
```go
registry.AddSerialized("crd__nodereadinessrules.yaml", nodeReadinessRuleCRDYAML)
```
Where `nodeReadinessRuleCRDYAML` is a `[]byte` constant containing the full CRD YAML fetched from the NRC repo at `v0.3.0`. This ensures the CRD is installed in the shoot before NRC starts and before the NodeReadinessRule CR is created.
