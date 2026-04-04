// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package criticalcomponents_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/gardener/gardener/pkg/resourcemanager/controller/node/criticalcomponents"
)

var _ = Describe("WriteNodeConditionCriticalComponentsReady", func() {
	var (
		testCtx = context.Background()
		node    *corev1.Node
	)

	BeforeEach(func() {
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		}
	})

	It("should set the CriticalComponentsReady condition to True on a node with no prior conditions", func() {
		fakeClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Node{}).
			WithObjects(node).
			Build()

		Expect(WriteNodeConditionCriticalComponentsReady(testCtx, fakeClient, node)).To(Succeed())

		updated := &corev1.Node{}
		Expect(fakeClient.Get(testCtx, client.ObjectKeyFromObject(node), updated)).To(Succeed())

		cond := findCriticalComponentsCondition(updated.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(corev1.ConditionTrue))
		Expect(cond.Reason).To(Equal("AllCriticalComponentsReady"))
		Expect(cond.LastTransitionTime).NotTo(BeZero())
	})

	It("should be a no-op if condition is already True", func() {
		// Truncate to seconds since JSON serialization loses sub-second precision.
		transitionTime := metav1.NewTime(time.Now().Truncate(time.Second))
		node.Status.Conditions = []corev1.NodeCondition{{
			Type:               corev1.NodeConditionType(v1beta1constants.NodeConditionCriticalComponentsReady),
			Status:             corev1.ConditionTrue,
			LastTransitionTime: transitionTime,
		}}
		fakeClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Node{}).
			WithObjects(node).
			Build()

		Expect(WriteNodeConditionCriticalComponentsReady(testCtx, fakeClient, node)).To(Succeed())

		updated := &corev1.Node{}
		Expect(fakeClient.Get(testCtx, client.ObjectKeyFromObject(node), updated)).To(Succeed())
		cond := findCriticalComponentsCondition(updated.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		// Transition time should be unchanged (no-op path)
		Expect(cond.LastTransitionTime).To(Equal(transitionTime))
	})

	It("should update the condition and preserve LastTransitionTime when status changes", func() {
		// Truncate to seconds since JSON serialization loses sub-second precision.
		originalTransitionTime := metav1.NewTime(time.Now().Add(-5 * time.Minute).Truncate(time.Second))
		node.Status.Conditions = []corev1.NodeCondition{{
			Type:               corev1.NodeConditionType(v1beta1constants.NodeConditionCriticalComponentsReady),
			Status:             corev1.ConditionFalse,
			LastTransitionTime: originalTransitionTime,
		}}
		fakeClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&corev1.Node{}).
			WithObjects(node).
			Build()

		Expect(WriteNodeConditionCriticalComponentsReady(testCtx, fakeClient, node)).To(Succeed())

		updated := &corev1.Node{}
		Expect(fakeClient.Get(testCtx, client.ObjectKeyFromObject(node), updated)).To(Succeed())
		cond := findCriticalComponentsCondition(updated.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(corev1.ConditionTrue))
		// LastTransitionTime should be preserved from original
		Expect(cond.LastTransitionTime).To(Equal(originalTransitionTime))
	})
})

func findCriticalComponentsCondition(conditions []corev1.NodeCondition) *corev1.NodeCondition {
	for i := range conditions {
		if conditions[i].Type == corev1.NodeConditionType(v1beta1constants.NodeConditionCriticalComponentsReady) {
			return &conditions[i]
		}
	}
	return nil
}
