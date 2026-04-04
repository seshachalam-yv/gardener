// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodereadinesscontroller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/utils/managedresources"
)

const (
	// ManagedResourceName is the name of the ManagedResource containing the resource specifications.
	ManagedResourceName    = "shoot-core-node-readiness-controller"
	serviceAccountName     = "node-readiness-controller"
	containerName          = "node-readiness-controller"
	deploymentName         = "node-readiness-controller"
	clusterRoleName        = "node-readiness-controller"
	clusterRoleBindingName = "node-readiness-controller"
	labelValue             = "node-readiness-controller"

	// nrcAPIGroup is the API group for Node Readiness Controller custom resources.
	nrcAPIGroup = "readiness.node.x-k8s.io"
	// nrcAPIVersion is the API version for NodeReadinessRule CR.
	nrcAPIVersion = "readiness.node.x-k8s.io/v1alpha1"
	// nrcRuleKind is the kind for NodeReadinessRule CR.
	nrcRuleKind = "NodeReadinessRule"
	// nrcRuleName is the name for the gardener NodeReadinessRule CR.
	nrcRuleName = "gardener-critical-components"
)

// Values is a set of configuration values for the node-readiness-controller component.
type Values struct {
	// Image is the container image used for node-readiness-controller.
	Image string
}

// New creates a new instance of DeployWaiter for node-readiness-controller.
func New(
	client client.Client,
	namespace string,
	values Values,
) component.DeployWaiter {
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

func (c *nodeReadinessController) Deploy(ctx context.Context) error {
	data, err := c.computeResourcesData()
	if err != nil {
		return err
	}

	return managedresources.CreateForShoot(ctx, c.client, c.namespace, ManagedResourceName, managedresources.LabelValueGardener, false, data)
}

func (c *nodeReadinessController) Destroy(ctx context.Context) error {
	return managedresources.DeleteForShoot(ctx, c.client, c.namespace, ManagedResourceName)
}

// TimeoutWaitForManagedResource is the timeout used while waiting for the ManagedResources to become healthy
// or deleted.
var TimeoutWaitForManagedResource = 2 * time.Minute

func (c *nodeReadinessController) Wait(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForManagedResource)
	defer cancel()

	return managedresources.WaitUntilHealthy(timeoutCtx, c.client, c.namespace, ManagedResourceName)
}

func (c *nodeReadinessController) WaitCleanup(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForManagedResource)
	defer cancel()

	return managedresources.WaitUntilDeleted(timeoutCtx, c.client, c.namespace, ManagedResourceName)
}

func (c *nodeReadinessController) computeResourcesData() (map[string][]byte, error) {
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
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
			{
				APIGroups: []string{nrcAPIGroup},
				Resources: []string{"nodereadinessrules"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{nrcAPIGroup},
				Resources: []string{"nodereadinessrules/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
		},
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        clusterRoleBindingName,
			Annotations: map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"},
			Labels:      getLabels(),
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

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: metav1.NamespaceSystem,
			Labels:    getLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(),
			},
			RevisionHistoryLimit: ptr.To[int32](2),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getLabels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount.Name,
					PriorityClassName:  v1beta1constants.PriorityClassNameShootSystem900,
					Tolerations: []corev1.Toleration{
						{
							Key:      v1beta1constants.TaintNodeCriticalComponentsNotReady,
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      v1beta1constants.TaintNodeReadinessControllerNotReady,
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: corev1.TolerationOpExists,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            containerName,
							Image:           c.values.Image,
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

	if err := registry.Add(serviceAccount, clusterRole, clusterRoleBinding, deployment); err != nil {
		return nil, err
	}

	// NodeReadinessRule CR is not a vendored type; add it as raw YAML using AddSerialized.
	// The filename follows the convention: <group>/<version>/<kind>__<namespace>__<name>.yaml
	// For cluster-scoped resources namespace is empty.
	nrcRuleYAML := fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: %s
spec:
  conditions:
  - type: "%s"
    requiredStatus: "True"
  taint:
    key: "%s"
    effect: "NoSchedule"
  enforcementMode: "bootstrap-only"
`,
		nrcAPIVersion,
		nrcRuleKind,
		nrcRuleName,
		v1beta1constants.NodeConditionCriticalComponentsReady,
		v1beta1constants.TaintNodeReadinessControllerNotReady,
	)

	registry.AddSerialized(
		fmt.Sprintf("%s/%s/%s____%s.yaml", nrcAPIGroup, "v1alpha1", nrcRuleKind, nrcRuleName),
		[]byte(nrcRuleYAML),
	)

	return registry.SerializedObjects()
}

func getLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     labelValue,
		"app.kubernetes.io/instance": "shoot-core",
	}
}
