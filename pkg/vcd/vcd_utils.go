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

package vcd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/peterhellberg/link"

	"strings"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getUserCredentialsForCluster(ctx context.Context, cli client.Client, definedCreds capvcd.UserCredentialsContext) (capvcd.UserCredentialsContext, error) {
	username, password, refreshToken := definedCreds.Username, definedCreds.Password, definedCreds.RefreshToken
	if definedCreds.SecretRef != nil {
		secretNamespacedName := types.NamespacedName{
			Name:      definedCreds.SecretRef.Name,
			Namespace: definedCreds.SecretRef.Namespace,
		}
		userCredsSecret := &v1.Secret{}
		if err := cli.Get(ctx, secretNamespacedName, userCredsSecret); err != nil {
			return capvcd.UserCredentialsContext{}, errors.Wrapf(err, "error getting secret [%s] in namespace [%s]",
				secretNamespacedName.Name, secretNamespacedName.Namespace)
		}
		if b, exists := userCredsSecret.Data["username"]; exists {
			username = strings.TrimRight(string(b), "\n")
		}
		if b, exists := userCredsSecret.Data["password"]; exists {
			password = strings.TrimRight(string(b), "\n")
		}
		if b, exists := userCredsSecret.Data["refreshToken"]; exists {
			refreshToken = strings.TrimRight(string(b), "\n")
		}
	}
	userCredentials := capvcd.UserCredentialsContext{
		Username:     username,
		Password:     password,
		RefreshToken: refreshToken,
	}

	return userCredentials, nil
}

// GetVCDClient a helper function for initializing vcd api client, it gets the credentials from the k8s secret
func GetVCDClient(ctx context.Context, c client.Client, vcdCluster *capvcd.VCDCluster, log logr.Logger) (*vcdsdk.Client, error) {
	userCreds, err := getUserCredentialsForCluster(ctx, c, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		log.V(1).Info("Error getting client credentials for vcd client", "vcdCluster", vcdCluster)
		return nil, microerror.Mask(err)
	}
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, userCreds.Username, userCreds.Password, userCreds.RefreshToken, true, true)
	if err != nil {
		log.V(1).Info("Error creating VCD client", "vcdCluster", vcdCluster)
		return nil, microerror.Mask(err)
	}
	return workloadVCDClient, nil
}

// GetGateway a helper function that creates and returns GatewayManager
func GetGateway(ctx context.Context, vcdClient *vcdsdk.Client, vcdCluster *capvcd.VCDCluster) (*vcdsdk.GatewayManager, error) {
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, vcdCluster.Spec.OvdcNetwork, vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		return nil, err
	}
	return gateway, nil
}

// GetCursor handles the paging mechanism for queries that may return a lot of items
// https://github.com/vmware/cloud-provider-for-cloud-director/blob/v1.2.0/pkg/vcdsdk/gateway.go#L199
func GetCursor(resp *http.Response) (string, error) {
	cursorURI := ""
	for _, linklet := range resp.Header["Link"] {
		for _, l := range link.Parse(linklet) {
			if l.Rel == "nextPage" {
				cursorURI = l.URI
				break
			}
		}
		if cursorURI != "" {
			break
		}
	}
	if cursorURI == "" {
		return "", nil
	}

	u, err := url.Parse(cursorURI)
	if err != nil {
		return "", fmt.Errorf("unable to parse cursor URI [%s]: [%v]", cursorURI, err)
	}

	cursorStr := ""
	keyMap, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("unable to parse raw query [%s]: [%v]", u.RawQuery, err)
	}

	if cursorStrList, ok := keyMap["cursor"]; ok {
		cursorStr = cursorStrList[0]
	}

	return cursorStr, nil
}
