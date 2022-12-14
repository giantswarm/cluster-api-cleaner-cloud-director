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

	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VirtualServiceCleaner struct {
	cli client.Client
}

func NewVirtualServiceCleaner(cli client.Client) *VirtualServiceCleaner {
	return &VirtualServiceCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &VirtualServiceCleaner{}

func (lbc *VirtualServiceCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (bool, error) {
	log = log.WithName("VirtualServiceCleaner")
	gateway, err := vcd.GetGateway(ctx, vcdClient, c)
	if err != nil {
		return false, err
	}
	vSvcs, err := vcdClient.VCDClient.GetAllAlbVirtualServices(gateway.GatewayRef.Id, nil)
	if err != nil {
		return false, err
	}
	infraId := c.Status.InfraId
	deleted := 0
	for _, vSvc := range vSvcs {
		svcName := vSvc.NsxtAlbVirtualService.Name
		// if the name of the virtual service contains the cluster's infraId, delete this virtual service
		if strings.Contains(svcName, infraId) {
			log.Info(fmt.Sprintf("deleting virtual service: %s", svcName))
			err = gateway.DeleteVirtualService(ctx, svcName, false)
			if err != nil {
				return false, err
			}
			deleted++
		}
	}
	if deleted > 0 {
		log.Info(fmt.Sprintf("%d virtual services were deleted", deleted))
	}

	return false, nil
}
