package cleaner

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
)

type Cleaner interface {
	Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (requeue bool, err error)
}
