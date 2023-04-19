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

	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"
)

type VolumeCleaner struct {
	cli client.Client
}

func NewVolumeCleaner(cli client.Client) *VolumeCleaner {
	return &VolumeCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &VolumeCleaner{}

func (vc *VolumeCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, cluster *capvcd.VCDCluster) (bool, error) {
	log = log.WithName("VolumeCleaner")

	diskRecords, err := vcd.GetDiskRecordsOfClusterByDescription(vcdClient, cluster.Status.InfraId)
	if err != nil {
		return false, fmt.Errorf("failed to get disk records of cluster:[%s] [%v]", cluster.Status.InfraId, err)
	}

	log.Info(fmt.Sprintf("%d disks will be deleted", len(diskRecords)))

	for _, diskRecord := range diskRecords {
		log.Info(fmt.Sprintf("Disk [%s] will be deleted", diskRecord.Name))

		disk, err := vcd.GetDiskByHref(vcdClient, diskRecord.HREF)
		if err != nil {
			return false, fmt.Errorf("failed to get disk:[%s] [%v]", diskRecord.Name, err)
		}

		err = vcd.DetachFromAllVms(vcdClient, cluster.Name, disk, log)
		if err != nil {
			return false, fmt.Errorf("failed to detach VMs from disk:[%s] [%v]", diskRecord.Name, err)
		}

		err = vcd.DeleteDisk(vcdClient, disk)
		if err != nil {
			return false, fmt.Errorf("failed to delete disk:[%s] [%v]", diskRecord.Name, err)
		}
	}

	return false, nil
}
