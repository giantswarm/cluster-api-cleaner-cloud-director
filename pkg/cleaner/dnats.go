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

type DNATCleaner struct {
	cli client.Client
}

func NewDNATCleaner(cli client.Client) *DNATCleaner {
	return &DNATCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &DNATCleaner{}

func (lbc *DNATCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (bool, error) {
	log = log.WithName("DNATCleaner")
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, c.Spec.OvdcNetwork, c.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		return false, err
	}
	if err := vcdClient.RefreshBearerToken(); err != nil {
		return false, err
	}
	org, err := vcdClient.VCDClient.GetOrgByName(vcdClient.ClusterOrgName)
	if err != nil {
		return false, microerror.Mask(err)
	}
	if org == nil || org.Org == nil {
		return false, microerror.Mask(fmt.Errorf("obtained nil org when getting org by name [%s]", vcdClient.ClusterOrgName))
	}
	edgeNatRules, _, err := vcdClient.APIClient.EdgeGatewayNatRulesApi.GetNatRules(
		ctx,
		128,
		gateway.GatewayRef.Id,
		org.Org.ID,
		nil)
	if err != nil {
		return false, err
	}
	infraId := c.Status.InfraId
	if len(infraId) == 0 {
		return true, microerror.Mask(fmt.Errorf(".status.infraId is not populated on the cluster: %s", c.Name))
	}
	if len(edgeNatRules.Values) == 0 {
		log.Info("there is nothing to do")
		return false, nil
	}
	deleted := 0

	for _, enr := range edgeNatRules.Values {
		enrName := enr.Name
		// if the name of the DNAT rule contains the cluster's infraId, delete this item
		if strings.Contains(enrName, infraId) {
			log.Info(fmt.Sprintf("deleting DNAT: %s", enrName))
			err = gateway.DeleteDNATRule(ctx, enrName, false)
			if err != nil {
				return false, err
			}
			deleted++
		}
	}
	if deleted > 0 {
		log.Info(fmt.Sprintf("%d DNATs were deleted", deleted))
	}

	return false, nil
}
