//go:build integration

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

package integration

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/test/integration/vcdfake"
)

// TestGetVCDClientFromSecret checks the whole credential path: read the secret
// from the api server, then log in to VCD with what it holds.
func TestGetVCDClientFromSecret(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	server := newVCDServer(t, vcdfake.Config{})

	createSecret(t, ctx, name, map[string][]byte{
		"username": []byte(testUsername),
		"password": []byte(testPassword),
	})

	vcdCluster := newVCDCluster(name, server.URL(), nil)
	vcdCluster.Spec.UserCredentialsContext = credentialsFromSecret(name)
	createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	client, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(client.ClusterOrgName).To(gomega.Equal(testOrgName))
	g.Expect(client.ClusterOVDCName).To(gomega.Equal(testVdcName))

	user, password := server.Credentials()
	g.Expect(user).To(gomega.Equal(testUsername))
	g.Expect(password).To(gomega.Equal(testPassword))
}

// TestGetVCDClientTrimsSecretNewlines checks that a secret written with a
// trailing newline still authenticates. `echo -n` is easy to forget.
func TestGetVCDClientTrimsSecretNewlines(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	server := newVCDServer(t, vcdfake.Config{})

	createSecret(t, ctx, name, map[string][]byte{
		"username": []byte(testUsername + "\n"),
		"password": []byte(testPassword + "\n"),
	})

	vcdCluster := newVCDCluster(name, server.URL(), nil)
	vcdCluster.Spec.UserCredentialsContext = credentialsFromSecret(name)
	createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	_, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// The fake rejects the login unless it receives the trimmed values.
	user, password := server.Credentials()
	g.Expect(user).To(gomega.Equal(testUsername))
	g.Expect(password).To(gomega.Equal(testPassword))
}

// TestGetVCDClientFromInlineCredentials checks the path without a secret ref.
func TestGetVCDClientFromInlineCredentials(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	server := newVCDServer(t, vcdfake.Config{})

	vcdCluster := newVCDCluster(name, server.URL(), nil)
	createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	_, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	user, password := server.Credentials()
	g.Expect(user).To(gomega.Equal(testUsername))
	g.Expect(password).To(gomega.Equal(testPassword))
}

// TestGetVCDClientWithMissingSecret checks the error names the secret, so an
// operator can find the problem from the log.
func TestGetVCDClientWithMissingSecret(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	server := newVCDServer(t, vcdfake.Config{})

	vcdCluster := newVCDCluster(name, server.URL(), nil)
	vcdCluster.Spec.UserCredentialsContext = credentialsFromSecret("no-such-secret")
	createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	_, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("no-such-secret"))
}

// TestGetVCDClientWithWrongPassword checks that a rejected login is an error
// rather than a client that fails later.
func TestGetVCDClientWithWrongPassword(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	server := newVCDServer(t, vcdfake.Config{})

	vcdCluster := newVCDCluster(name, server.URL(), nil)
	vcdCluster.Spec.UserCredentialsContext.Password = "wrong-password"
	createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	_, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).To(gomega.HaveOccurred())
}

// credentialsFromSecret points the cluster at a credentials secret.
func credentialsFromSecret(name string) capvcd.UserCredentialsContext {
	return capvcd.UserCredentialsContext{
		SecretRef: &corev1.SecretReference{
			Name:      name,
			Namespace: testNamespace,
		},
	}
}
