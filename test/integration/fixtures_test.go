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
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/key"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/test/integration/vcdfake"
)

const (
	testNamespace = "default"

	// The VCD identifiers the fake VCD server is configured with.
	testOrgName     = "test-org"
	testVdcName     = "test-ovdc"
	testNetworkName = "test-network"
	testUsername    = "test-user"
	testPassword    = "test-password"

	// stubCleanerName labels the cleaner used when a test needs only one.
	stubCleanerName = "stub"
)

// uniqueName keeps objects from different test cases apart. envtest has no
// namespace controller, so deleting namespaces between tests does not work.
func uniqueName(t *testing.T) string {
	t.Helper()

	name := ""
	for _, r := range t.Name() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			name += string(r)
		case r >= 'A' && r <= 'Z':
			name += string(r + 32)
		default:
			name += "-"
		}
	}

	return name
}

// newCluster builds a capi Cluster. Cluster.Spec is required by the crd but its
// json tag is `omitempty,omitzero`, so an empty spec fails to create.
func newCluster(name string) *capi.Cluster {
	return &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: capi.ClusterSpec{
			Paused: ptr.To(false),
		},
	}
}

// newVCDCluster builds a VCDCluster owned by the given capi Cluster. site is the
// VCD endpoint, which the tests point at the fake VCD server.
func newVCDCluster(name, site string, owner *capi.Cluster) *capvcd.VCDCluster {
	vcdCluster := &capvcd.VCDCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				key.CapiClusterLabelKey: name,
			},
		},
		Spec: capvcd.VCDClusterSpec{
			Site:        site,
			Org:         testOrgName,
			Ovdc:        testVdcName,
			OvdcNetwork: testNetworkName,
			UserCredentialsContext: capvcd.UserCredentialsContext{
				Username: testUsername,
				Password: testPassword,
			},
		},
	}

	if owner != nil {
		vcdCluster.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: capi.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       owner.Name,
			UID:        owner.UID,
		}}
	}

	return vcdCluster
}

// createCluster creates the capi Cluster and registers its cleanup.
func createCluster(t *testing.T, ctx context.Context, cluster *capi.Cluster) *capi.Cluster {
	t.Helper()
	g := gomega.NewWithT(t)

	g.Expect(k8sClient.Create(ctx, cluster)).To(gomega.Succeed())
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), cluster)
	})

	return cluster
}

// createVCDCluster creates the VCDCluster. infraId is written through the status
// subresource, because a Create with a populated status silently drops it.
func createVCDCluster(t *testing.T, ctx context.Context, vcdCluster *capvcd.VCDCluster, infraId string) *capvcd.VCDCluster {
	t.Helper()
	g := gomega.NewWithT(t)

	g.Expect(k8sClient.Create(ctx, vcdCluster)).To(gomega.Succeed())
	t.Cleanup(func() {
		cleanupVCDCluster(vcdCluster)
	})

	if infraId != "" {
		vcdCluster.Status.InfraId = infraId
		vcdCluster.Status.Org = testOrgName
		g.Expect(k8sClient.Status().Update(ctx, vcdCluster)).To(gomega.Succeed())
	}

	return vcdCluster
}

// cleanupVCDCluster removes the object even when the test left the finalizer on
// it, so that a failing test does not block the next run.
func cleanupVCDCluster(vcdCluster *capvcd.VCDCluster) {
	ctx := context.Background()

	current := &capvcd.VCDCluster{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(vcdCluster), current); err != nil {
		return
	}

	if len(current.Finalizers) > 0 {
		current.Finalizers = nil
		_ = k8sClient.Update(ctx, current)
	}

	_ = k8sClient.Delete(ctx, current)
}

// getVCDCluster reads the object back from the api server.
func getVCDCluster(t *testing.T, ctx context.Context, name string) *capvcd.VCDCluster {
	t.Helper()
	g := gomega.NewWithT(t)

	vcdCluster := &capvcd.VCDCluster{}
	g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, vcdCluster)).To(gomega.Succeed())

	return vcdCluster
}

// createSecret creates a VCD credentials secret and registers its cleanup.
func createSecret(t *testing.T, ctx context.Context, name string, data map[string][]byte) *corev1.Secret {
	t.Helper()
	g := gomega.NewWithT(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: data,
	}

	g.Expect(k8sClient.Create(ctx, secret)).To(gomega.Succeed())
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), secret)
	})

	return secret
}

// newVCDServer starts a fake VCD endpoint. It fills in the identifiers the
// fixtures use, so a test only sets the resources it cares about.
func newVCDServer(t *testing.T, cfg vcdfake.Config) *vcdfake.Server {
	t.Helper()

	cfg.OrgName = testOrgName
	cfg.VdcName = testVdcName
	cfg.NetworkName = testNetworkName
	cfg.Username = testUsername
	cfg.Password = testPassword

	if cfg.VAppName == "" {
		cfg.VAppName = uniqueName(t)
	}

	server := vcdfake.New(cfg)
	t.Cleanup(server.Close)

	return server
}

// reconcileRequest builds the request for a VCDCluster.
func reconcileRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: testNamespace}}
}

// stubCleaner records the clusters it was asked to clean and returns a scripted
// result.
type stubCleaner struct {
	name    string
	requeue bool
	err     error

	calls []*capvcd.VCDCluster
	order *[]string
}

func (s *stubCleaner) Clean(ctx context.Context, log logr.Logger, vcdClient *vcdsdk.Client, c *capvcd.VCDCluster) (bool, error) {
	s.calls = append(s.calls, c.DeepCopy())
	if s.order != nil {
		*s.order = append(*s.order, s.name)
	}

	return s.requeue, s.err
}

// callCount reports how often the cleaner ran.
func (s *stubCleaner) callCount() int {
	return len(s.calls)
}

var errStubVCDClient = fmt.Errorf("stub vcd client failure")
