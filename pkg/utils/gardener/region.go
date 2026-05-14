// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gardener

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
)

// DefaultInterRegionDistanceThreshold is the default maximum allowed inter-region distance for live control plane migration.
const DefaultInterRegionDistanceThreshold = 180

// FindRegionConfigMap finds the scheduler region ConfigMap that matches the given cloud profile name
// from a list of region ConfigMaps.
func FindRegionConfigMap(regionConfigMaps []*corev1.ConfigMap, cloudProfileName string) *corev1.ConfigMap {
	for _, cm := range regionConfigMaps {
		for name := range strings.SplitSeq(cm.Annotations[v1beta1constants.AnnotationSchedulingCloudProfiles], ",") {
			if strings.TrimSpace(name) == cloudProfileName {
				return cm
			}
		}
	}
	return nil
}

// GetDistanceThreshold reads the inter-region distance threshold from the region ConfigMap annotation.
// If the annotation is not set, it returns DefaultInterRegionDistanceThreshold.
func GetDistanceThreshold(regionConfigMap *corev1.ConfigMap) (int, error) {
	thresholdStr, ok := regionConfigMap.Annotations[v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold]
	if !ok {
		return DefaultInterRegionDistanceThreshold, nil
	}

	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q for annotation %q on ConfigMap %s: %w",
			thresholdStr, v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold, client.ObjectKeyFromObject(regionConfigMap), err)
	}

	if threshold < 0 {
		return 0, fmt.Errorf("threshold must be non-negative, got %d for annotation %q on ConfigMap %s",
			threshold, v1beta1constants.AnnotationMigrationInterRegionDistanceThreshold, client.ObjectKeyFromObject(regionConfigMap))
	}

	return threshold, nil
}

// GetRegionDistance parses the region distances for the given source region from the region ConfigMap.
// It returns the distances map, whether the source region was found in the ConfigMap, and any parse error.
func GetRegionDistance(regionConfigMap *corev1.ConfigMap, sourceRegion string) (map[string]int, bool, error) {
	data := regionConfigMap.Data[sourceRegion]
	if data == "" {
		return nil, false, nil
	}

	regionDistances := make(map[string]int)
	if err := yaml.Unmarshal([]byte(data), &regionDistances); err != nil {
		return nil, false, fmt.Errorf("failed to parse region distances for region %q in ConfigMap %s: %w",
			sourceRegion, client.ObjectKeyFromObject(regionConfigMap), err)
	}

	return regionDistances, true, nil
}
