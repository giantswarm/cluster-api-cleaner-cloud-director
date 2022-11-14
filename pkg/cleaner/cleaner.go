package cleaner

import (
	"context"

	"github.com/go-logr/logr"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
)

type Cleaner interface {
	Clean(ctx context.Context, log logr.Logger, oc *capvcd.VCDCluster, clusterTag string) (requeue bool, err error)
}
