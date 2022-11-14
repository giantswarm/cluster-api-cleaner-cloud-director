package cleaner

import (
	"context"

	"github.com/go-logr/logr"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VolumeCleaner struct {
	cli client.Client
}

func NewVolumeCleaner(cli client.Client) *VolumeCleaner {
	return &VolumeCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &VolumeCleaner{}

func (vc *VolumeCleaner) Clean(ctx context.Context, log logr.Logger, c *capvcd.VCDCluster, clusterTag string) (bool, error) {
	log = log.WithName("VolumeCleaner")
	// todo: this
	return false, nil
}
