// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodereadinesscontroller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/component"
	. "github.com/gardener/gardener/pkg/component/nodemanagement/nodereadinesscontroller"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/garbagecollector/references"
	"github.com/gardener/gardener/pkg/utils/retry"
	retryfake "github.com/gardener/gardener/pkg/utils/retry/fake"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("NodeReadinessController", func() {
	var (
		ctx = context.Background()

		managedResourceName = "shoot-core-node-readiness-controller"
		namespace           = "some-namespace"
		image               = "some-nrc-image:tag"

		c                     client.Client
		comp                  component.DeployWaiter
		manifests             []string
		expectedManifests     []string
		managedResource       *resourcesv1alpha1.ManagedResource
		managedResourceSecret *corev1.Secret
		values                Values
	)

	BeforeEach(func() {
		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.SeedScheme).Build()
		values = Values{
			Image: image,
		}
		comp = New(c, namespace, values)

		managedResource = &resourcesv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedResourceName,
				Namespace: namespace,
			},
		}
		managedResourceSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managedresource-" + managedResource.Name,
				Namespace: namespace,
			},
		}
	})

	Describe("#Deploy", func() {
		var (
			serviceAccountYAML = `apiVersion: v1
automountServiceAccountToken: false
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/instance: shoot-core
    app.kubernetes.io/name: node-readiness-controller
  name: node-readiness-controller
  namespace: kube-system
`
			clusterRoleYAML = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/instance: shoot-core
    app.kubernetes.io/name: node-readiness-controller
  name: node-readiness-controller
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
  - update
- apiGroups:
  - readiness.node.x-k8s.io
  resources:
  - nodereadinessrules
  verbs:
  - get
  - list
  - watch
  - update
  - patch
- apiGroups:
  - readiness.node.x-k8s.io
  resources:
  - nodereadinessrules/status
  verbs:
  - get
  - update
  - patch
`
			clusterRoleBindingYAML = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  annotations:
    resources.gardener.cloud/delete-on-invalid-update: "true"
  labels:
    app.kubernetes.io/instance: shoot-core
    app.kubernetes.io/name: node-readiness-controller
  name: node-readiness-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: node-readiness-controller
subjects:
- kind: ServiceAccount
  name: node-readiness-controller
  namespace: kube-system
`
			deploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/instance: shoot-core
    app.kubernetes.io/name: node-readiness-controller
  name: node-readiness-controller
  namespace: kube-system
spec:
  replicas: 1
  revisionHistoryLimit: 2
  selector:
    matchLabels:
      app.kubernetes.io/instance: shoot-core
      app.kubernetes.io/name: node-readiness-controller
  strategy: {}
  template:
    metadata:
      labels:
        app.kubernetes.io/instance: shoot-core
        app.kubernetes.io/name: node-readiness-controller
    spec:
      containers:
      - image: ` + image + `
        imagePullPolicy: IfNotPresent
        name: node-readiness-controller
        resources:
          limits:
            memory: 128Mi
          requests:
            cpu: 10m
            memory: 32Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
      priorityClassName: gardener-shoot-system-900
      serviceAccountName: node-readiness-controller
      tolerations:
      - effect: NoSchedule
        key: node.gardener.cloud/critical-components-not-ready
        operator: Exists
      - effect: NoSchedule
        key: readiness.k8s.io/gardener-critical-components-not-ready
        operator: Exists
status: {}
`
			nodeReadinessRuleYAML = `apiVersion: readiness.node.x-k8s.io/v1alpha1
kind: NodeReadinessRule
metadata:
  name: gardener-critical-components
spec:
  conditions:
  - type: "node.gardener.cloud/CriticalComponentsReady"
    requiredStatus: "True"
  taint:
    key: "readiness.k8s.io/gardener-critical-components-not-ready"
    effect: "NoSchedule"
  enforcementMode: "bootstrap-only"
`
		)

		JustBeforeEach(func() {
			comp = New(c, namespace, values)
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())
			Expect(comp.Deploy(ctx)).To(Succeed())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			expectedMr := &resourcesv1alpha1.ManagedResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:            managedResource.Name,
					Namespace:       managedResource.Namespace,
					ResourceVersion: "1",
					Labels:          map[string]string{"origin": "gardener"},
				},
				Spec: resourcesv1alpha1.ManagedResourceSpec{
					InjectLabels: map[string]string{"shoot.gardener.cloud/no-cleanup": "true"},
					SecretRefs: []corev1.LocalObjectReference{{
						Name: managedResource.Spec.SecretRefs[0].Name,
					}},
					KeepObjects: ptr.To(false),
				},
			}
			utilruntime.Must(references.InjectAnnotations(expectedMr))
			Expect(managedResource).To(DeepEqual(expectedMr))

			managedResourceSecret.Name = managedResource.Spec.SecretRefs[0].Name
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())
			Expect(managedResourceSecret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(managedResourceSecret.Immutable).To(Equal(ptr.To(true)))
			Expect(managedResourceSecret.Labels["resources.gardener.cloud/garbage-collectable-reference"]).To(Equal("true"))

			var err error
			manifests, err = test.ExtractManifestsFromManagedResourceData(managedResourceSecret.Data)
			Expect(err).NotTo(HaveOccurred())

			expectedManifests = []string{
				serviceAccountYAML,
				deploymentYAML,
				clusterRoleYAML,
				clusterRoleBindingYAML,
				nodeReadinessRuleYAML,
			}
		})

		It("should successfully deploy all resources", func() {
			Expect(manifests).To(ConsistOf(expectedManifests))
		})
	})

	Describe("#Destroy", func() {
		It("should successfully destroy all resources", func() {
			Expect(c.Create(ctx, managedResource)).To(Succeed())
			Expect(c.Create(ctx, managedResourceSecret)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())

			Expect(comp.Destroy(ctx)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(BeNotFoundError())
		})
	})

	Context("waiting functions", func() {
		var (
			fakeOps   *retryfake.Ops
			resetVars func()
		)

		BeforeEach(func() {
			fakeOps = &retryfake.Ops{MaxAttempts: 1}
			resetVars = test.WithVars(
				&retry.Until, fakeOps.Until,
				&retry.UntilTimeout, fakeOps.UntilTimeout,
			)
		})

		AfterEach(func() {
			resetVars()
		})

		Describe("#Wait", func() {
			It("should fail because reading the ManagedResource fails", func() {
				Expect(comp.Wait(ctx)).To(MatchError(ContainSubstring("not found")))
			})

			It("should fail because the ManagedResource doesn't become healthy", func() {
				oldTimeout := TimeoutWaitForManagedResource
				defer func() { TimeoutWaitForManagedResource = oldTimeout }()
				TimeoutWaitForManagedResource = time.Millisecond

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionFalse,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionFalse,
							},
						},
					},
				})).To(Succeed())

				Expect(comp.Wait(ctx)).To(MatchError(ContainSubstring("is not healthy")))
			})

			It("should successfully wait for the managed resource to become healthy", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionTrue,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionTrue,
							},
						},
					},
				})).To(Succeed())

				Expect(comp.Wait(ctx)).To(Succeed())
			})
		})

		Describe("#WaitCleanup", func() {
			It("should fail when the wait for the managed resource deletion times out", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, managedResource)).To(Succeed())

				Expect(comp.WaitCleanup(ctx)).To(MatchError(ContainSubstring("still exists")))
			})

			It("should not return an error when it's already removed", func() {
				Expect(comp.WaitCleanup(ctx)).To(Succeed())
			})
		})
	})
})
