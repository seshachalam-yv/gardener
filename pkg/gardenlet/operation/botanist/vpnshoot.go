// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package botanist

import (
	"github.com/gardener/gardener/imagevector"
	"github.com/gardener/gardener/pkg/component"
	vpnseedserver "github.com/gardener/gardener/pkg/component/networking/vpn/seedserver"
	vpnshoot "github.com/gardener/gardener/pkg/component/networking/vpn/shoot"
	imagevectorutils "github.com/gardener/gardener/pkg/utils/imagevector"
)

// DefaultVPNShoot returns a deployer for the VPNShoot
func (b *Botanist) DefaultVPNShoot() (component.DeployWaiter, error) {
	image, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpnShootClient, imagevectorutils.RuntimeVersion(b.ShootVersion()), imagevectorutils.TargetVersion(b.ShootVersion()))
	if err != nil {
		return nil, err
	}

	values := vpnshoot.Values{
		Image:             image.String(),
		VPAEnabled:        b.Shoot.WantsVerticalPodAutoscaler,
		VPAUpdateDisabled: b.Shoot.VPNVPAUpdateDisabled,
		ReversedVPN: vpnshoot.ReversedVPNValues{
			Header:      "outbound|1194||" + vpnseedserver.ServiceName + "." + b.Shoot.SeedNamespace + ".svc.cluster.local",
			Endpoint:    b.outOfClusterAPIServerFQDN(),
			OpenVPNPort: 8132,
			IPFamilies:  b.Shoot.GetInfo().Spec.Networking.IPFamilies,
		},
		HighAvailabilityEnabled:              b.Shoot.VPNHighAvailabilityEnabled,
		HighAvailabilityNumberOfSeedServers:  b.Shoot.VPNHighAvailabilityNumberOfSeedServers,
		HighAvailabilityNumberOfShootClients: b.Shoot.VPNHighAvailabilityNumberOfShootClients,
		KubernetesVersion:                    b.Shoot.KubernetesVersion,
	}

	return vpnshoot.New(
		b.SeedClientSet.Client(),
		b.Shoot.SeedNamespace,
		b.SecretsManager,
		values,
	), nil
}
