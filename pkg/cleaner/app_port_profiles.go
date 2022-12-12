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

	"github.com/vmware/go-vcloud-director/v2/types/v56"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"

	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AppPortProfileCleaner struct {
	cli client.Client
}

func NewAppPortProfileCleaner(cli client.Client) *AppPortProfileCleaner {
	return &AppPortProfileCleaner{cli: cli}
}

// force implementing Cleaner interface
var _ Cleaner = &AppPortProfileCleaner{}

func (lbc *AppPortProfileCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (bool, error) {
	log = log.WithName("AppPortProfileCleaner")
	gateway, err := vcd.GetGateway(ctx, vcdClient, c)
	if err != nil {
		return false, err
	}
	org, err := vcdClient.VCDClient.GetOrgByName(c.Status.Org)
	if err != nil {
		return false, err
	}
	aports, err := org.GetAllNsxtAppPortProfiles(nil, types.ApplicationPortProfileScopeTenant)
	if err != nil {
		return false, err
	}

	infraId := c.Status.InfraId
	deleted := 0
	for _, aport := range aports {
		aportName := aport.NsxtAppPortProfile.Name
		// if the name of the app port profile contains the cluster's infraId, delete it
		if strings.Contains(aportName, infraId) {
			log.Info(fmt.Sprintf("deleting app port profile: %s", aportName))
			err = gateway.DeleteAppPortProfile(aportName, false)
			if err != nil {
				return false, err
			}
			deleted++
		}
	}
	if deleted > 0 {
		log.Info(fmt.Sprintf("%d app port profiles were deleted", deleted))
	}

	return false, nil
}
