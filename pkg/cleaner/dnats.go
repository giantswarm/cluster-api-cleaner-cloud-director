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
	edgeNatRules, _, err := vcdClient.APIClient.EdgeGatewayNatRulesApi.GetNatRules(ctx, 128, gateway.GatewayRef.Id, c.Spec.Org, nil)
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
