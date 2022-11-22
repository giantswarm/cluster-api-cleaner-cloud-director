/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cleaner

import (
	"context"
	"fmt"
	"strings"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LBPoolCleaner struct {
	cli client.Client
}

func NewLBPoolCleaner(cli client.Client) *LBPoolCleaner {
	return &LBPoolCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &LBPoolCleaner{}

func (lbc *LBPoolCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (bool, error) {
	log = log.WithName("LBPoolCleaner")
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, c.Spec.OvdcNetwork, c.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		return false, err
	}
	lbps, err := vcdClient.VCDClient.GetAllAlbPools(gateway.GatewayRef.Id, nil)
	if err != nil {
		return false, err
	}
	infraId := c.Status.InfraId
	if len(infraId) == 0 {
		return true, microerror.Mask(fmt.Errorf(".status.infraId is not populated on the cluster: %s", c.Name))
	}
	deleted := 0
	for _, lbp := range lbps {
		lbName := lbp.NsxtAlbPool.Name
		// if the name of the load balancer pool contains the cluster's infraId, delete this lb pool
		if strings.Contains(lbName, infraId) {
			log.Info(fmt.Sprintf("deleting load balancer pool: %s", lbName))
			err = gateway.DeleteLoadBalancerPool(ctx, lbName, false)
			if err != nil {
				return false, err
			}
			deleted++
		}
	}
	if deleted > 0 {
		log.Info(fmt.Sprintf("%d load balancer pool were deleted", deleted))
	}

	return false, nil
}
