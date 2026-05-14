// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gardener_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/gardener/gardener/pkg/utils/gardener"
)

var _ = Describe("Region", func() {
	var configMap *corev1.ConfigMap

	BeforeEach(func() {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "region-config",
				Namespace: "garden",
				Annotations: map[string]string{
					v1beta1constants.AnnotationSchedulingCloudProfiles: "aws-profile,gcp-profile",
				},
			},
			Data: map[string]string{
				"eu-west-1": "eu-central-1: 50\nus-east-1: 200",
			},
		}
	})

	Describe("#FindRegionConfigMap", func() {
		var configMaps []*corev1.ConfigMap

		BeforeEach(func() {
			configMaps = []*corev1.ConfigMap{
				configMap,
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "region-config-2",
						Namespace: "garden",
						Annotations: map[string]string{
							v1beta1constants.AnnotationSchedulingCloudProfiles: "azure-profile",
						},
					},
				},
			}
		})

		It("should find the ConfigMap matching the cloud profile name", func() {
			cm := FindRegionConfigMap(configMaps, "aws-profile")
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal("region-config"))
		})

		It("should find the ConfigMap for second entry in comma-separated list", func() {
			cm := FindRegionConfigMap(configMaps, "gcp-profile")
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal("region-config"))
		})

		It("should return nil when no ConfigMap matches", func() {
			cm := FindRegionConfigMap(configMaps, "unknown-profile")
			Expect(cm).To(BeNil())
		})

		It("should handle empty list", func() {
			cm := FindRegionConfigMap(nil, "aws-profile")
			Expect(cm).To(BeNil())
		})

		It("should trim spaces in cloud profile names", func() {
			configMap.Annotations[v1beta1constants.AnnotationSchedulingCloudProfiles] = "aws-profile , gcp-profile"
			cm := FindRegionConfigMap(configMaps, "gcp-profile")
			Expect(cm).NotTo(BeNil())
			Expect(cm.Name).To(Equal("region-config"))
		})
	})

	Describe("#GetDistanceThreshold", func() {
		It("should return the default threshold when annotation is not set", func() {
			delete(configMap.Annotations, v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold)
			threshold, err := GetDistanceThreshold(configMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(threshold).To(Equal(DefaultInterRegionDistanceThreshold))
		})

		It("should return the threshold from annotation", func() {
			configMap.Annotations[v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold] = "100"
			threshold, err := GetDistanceThreshold(configMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(threshold).To(Equal(100))
		})

		It("should return an error for invalid threshold value", func() {
			configMap.Annotations[v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold] = "invalid"
			_, err := GetDistanceThreshold(configMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid value"))
		})

		It("should return an error for negative threshold value", func() {
			configMap.Annotations[v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold] = "-1"
			_, err := GetDistanceThreshold(configMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be non-negative"))
		})
	})

	Describe("#GetRegionDistance", func() {
		It("should parse region distances correctly", func() {
			distances, found, err := GetRegionDistance(configMap, "eu-west-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(distances).To(HaveKeyWithValue("eu-central-1", 50))
			Expect(distances).To(HaveKeyWithValue("us-east-1", 200))
		})

		It("should return not found for missing source region", func() {
			distances, found, err := GetRegionDistance(configMap, "ap-southeast-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(distances).To(BeNil())
		})

		It("should return an error for invalid YAML", func() {
			configMap.Data["eu-west-1"] = "invalid: yaml: data"
			_, _, err := GetRegionDistance(configMap, "eu-west-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse region distances"))
		})
	})
})
