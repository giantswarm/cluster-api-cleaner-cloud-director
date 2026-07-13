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

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"

	"github.com/antihax/optional"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
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
	gateway, err := vcd.GetGateway(ctx, vcdClient, c)
	if err != nil {
		return false, err
	}
	org, err := vcdClient.VCDClient.GetOrgByName(vcdClient.ClusterOrgName)
	if err != nil {
		return false, microerror.Mask(err)
	}
	if org == nil || org.Org == nil {
		return false, microerror.Mask(fmt.Errorf("obtained nil org when getting org by name [%s]", vcdClient.ClusterOrgName))
	}
	var toDelete []string
	infraId := c.Status.InfraId
	cursor := optional.EmptyString()

	// in each iteration we will be fetching 128 nat rules
	for {
		edgeNatRules, resp, err := vcdClient.APIClient.EdgeGatewayNatRulesApi.GetNatRules(
			ctx,
			128,
			gateway.GatewayRef.Id,
			org.Org.ID,
			&swaggerClient.EdgeGatewayNatRulesApiGetNatRulesOpts{
				Cursor: cursor,
			})
		if err != nil {
			return false, err
		}

		for _, enr := range edgeNatRules.Values {
			enrName := enr.Name
			// if the name of the DNAT rule contains the cluster's infraId, delete this item
			if strings.Contains(enrName, infraId) {
				toDelete = append(toDelete, enrName)
			}
		}
		cursorStr, err := vcd.GetCursor(resp)
		if err != nil {
			return false, microerror.Mask(fmt.Errorf("error while parsing response [%+v]: [%v]", resp, err))
		}
		if cursorStr == "" {
			break
		}
		cursor = optional.NewString(cursorStr)
	}

	// do the actual deletion, it needs to be done in a separate step, otherwise the paging&cursor is not behaving correctly
	if len(toDelete) > 0 {
		for _, enr := range toDelete {
			log.Info(fmt.Sprintf("deleting DNAT: %s", enr))
			err = gateway.DeleteDNATRule(ctx, enr, false)
			if err != nil {
				return false, err
			}
		}
		log.Info(fmt.Sprintf("%d DNATs were deleted", len(toDelete)))
	}

	return false, nil
}
