package cleaner

import (
	"context"

	"github.com/go-logr/logr"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoadBalancerCleaner struct {
	cli client.Client
}

func NewLoadBalancerCleaner(cli client.Client) *LoadBalancerCleaner {
	return &LoadBalancerCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &LoadBalancerCleaner{}

func (lbc *LoadBalancerCleaner) Clean(ctx context.Context, log logr.Logger, c *capvcd.VCDCluster, clusterTag string) (bool, error) {
	log = log.WithName("LoadBalancerCleaner")
	// todo: this
	return false, nil
}
