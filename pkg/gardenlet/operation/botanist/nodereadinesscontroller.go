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
)

// DefaultNodeReadinessController returns a deployer for the Node Readiness Controller.
// Returns a no-op deployer if the NodeReadinessController feature gate is disabled.
func (b *Botanist) DefaultNodeReadinessController() (component.DeployWaiter, error) {
	if !features.DefaultFeatureGate.Enabled(features.NodeReadinessController) {
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
