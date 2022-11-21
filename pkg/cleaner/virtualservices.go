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
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, c.Spec.OvdcNetwork, c.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		return false, err
	}
	vSvcs, err := vcdClient.VCDClient.GetAllAlbVirtualServices(gateway.GatewayRef.Id, nil)
	if err != nil {
		return false, err
	}
	infraId := c.Status.InfraId
	if len(infraId) == 0 {
		return true, microerror.Mask(fmt.Errorf(".status.infraId is not populated on the cluster: %s", c.Name))
	}
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
