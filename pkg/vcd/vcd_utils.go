package vcd

import (
	"context"

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
