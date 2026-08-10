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
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/controllers"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/cleaner"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/key"
)

// newReconciler builds a reconciler that never talks to a real VCD endpoint.
func newReconciler(cleaners []*stubCleaner) *controllers.VCDClusterReconciler {
	list := make([]cleaner.Cleaner, 0, len(cleaners))
	for _, c := range cleaners {
		list = append(list, c)
	}

	return &controllers.VCDClusterReconciler{
		Client:   k8sClient,
		Log:      logr.Discard(),
		Cleaners: list,
		NewVCDClient: func(ctx context.Context, c client.Client, vcdCluster *capvcd.VCDCluster, log logr.Logger) (*vcdsdk.Client, error) {
			return &vcdsdk.Client{}, nil
		},
	}
}

func TestReconcileAddsFinalizer(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "")

	r := newReconciler(nil)

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.ContainElement(key.CleanerFinalizerName))
}

func TestReconcileIsIdempotent(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "")

	r := newReconciler(nil)

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	first := getVCDCluster(t, ctx, name).ResourceVersion

	_, err = r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// A second pass must not write the object again.
	g.Expect(getVCDCluster(t, ctx, name).ResourceVersion).To(gomega.Equal(first))
}

func TestReconcileWithoutOwnerReference(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", nil), "")

	r := newReconciler(nil)

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.BeEmpty())
}

func TestReconcileSkipsPausedCluster(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := newCluster(name)
	cluster.Spec.Paused = ptr.To(true)
	cluster = createCluster(t, ctx, cluster)

	createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "")

	r := newReconciler(nil)

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.BeEmpty())
}

func TestReconcileMissingObject(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	r := newReconciler(nil)

	result, err := r.Reconcile(ctx, reconcileRequest("does-not-exist"))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())
}

func TestReconcileDeleteWithoutInfraId(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "")

	stub := &stubCleaner{name: stubCleanerName}
	r := newReconciler([]*stubCleaner{stub})

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	// Creation is assumed to have failed, so nothing is cleaned up.
	g.Expect(stub.callCount()).To(gomega.Equal(0))
	g.Expect(vcdClusterIsGone(ctx, name)).To(gomega.BeTrue())
}

func TestReconcileDeleteRunsCleaners(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "infra-"+name)

	order := []string{}
	stubs := []*stubCleaner{
		{name: "volumes", order: &order},
		{name: "virtualservices", order: &order},
		{name: "lbpools", order: &order},
		{name: "dnats", order: &order},
		{name: "appportprofiles", order: &order},
	}
	r := newReconciler(stubs)

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	// Every cleaner runs once, in the order the reconciler was given them.
	g.Expect(order).To(gomega.Equal([]string{"volumes", "virtualservices", "lbpools", "dnats", "appportprofiles"}))
	for _, stub := range stubs {
		g.Expect(stub.callCount()).To(gomega.Equal(1))
		g.Expect(stub.calls[0].Status.InfraId).To(gomega.Equal("infra-" + name))
	}

	g.Expect(vcdClusterIsGone(ctx, name)).To(gomega.BeTrue())
}

func TestReconcileDeleteRequeuesWhileCleanupRuns(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "infra-"+name)

	pending := &stubCleaner{name: "pending", requeue: true}
	done := &stubCleaner{name: "done"}
	r := newReconciler([]*stubCleaner{pending, done})

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.RequeueAfter).To(gomega.Equal(10 * time.Second))

	// A requeue must not drop the finalizer, otherwise the object disappears
	// while VCD resources still exist.
	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.ContainElement(key.CleanerFinalizerName))

	// Every cleaner still runs, even after one of them asked for a requeue.
	g.Expect(done.callCount()).To(gomega.Equal(1))

	// Once the clean-up finishes the finalizer goes.
	pending.requeue = false

	result, err = r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())
	g.Expect(vcdClusterIsGone(ctx, name)).To(gomega.BeTrue())
}

func TestReconcileDeleteKeepsFinalizerOnCleanerError(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "infra-"+name)

	failing := &stubCleaner{name: "failing", err: errStubVCDClient}
	never := &stubCleaner{name: "never"}
	r := newReconciler([]*stubCleaner{failing, never})

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	_, err = r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).To(gomega.HaveOccurred())

	// The first error stops the loop, so the later cleaners do not run.
	g.Expect(never.callCount()).To(gomega.Equal(0))
	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.ContainElement(key.CleanerFinalizerName))
}

func TestReconcileDeleteWithoutClusterNameLabel(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := newVCDCluster(name, "https://vcd.invalid", cluster)
	delete(vcdCluster.Labels, key.CapiClusterLabelKey)
	vcdCluster = createVCDCluster(t, ctx, vcdCluster, "infra-"+name)

	stub := &stubCleaner{name: stubCleanerName}
	r := newReconciler([]*stubCleaner{stub})

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	result, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())

	// Current behaviour: without the label the object stays terminating for ever
	// and no clean-up happens.
	g.Expect(stub.callCount()).To(gomega.Equal(0))
	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.ContainElement(key.CleanerFinalizerName))
}

func TestReconcileDeleteSwallowsVCDClientError(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	cluster := createCluster(t, ctx, newCluster(name))
	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, "https://vcd.invalid", cluster), "infra-"+name)

	stub := &stubCleaner{name: stubCleanerName}
	r := newReconciler([]*stubCleaner{stub})

	_, err := r.Reconcile(ctx, reconcileRequest(name))
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(k8sClient.Delete(ctx, vcdCluster)).To(gomega.Succeed())

	r.NewVCDClient = func(ctx context.Context, c client.Client, vcdCluster *capvcd.VCDCluster, log logr.Logger) (*vcdsdk.Client, error) {
		return nil, errStubVCDClient
	}

	result, err := r.Reconcile(ctx, reconcileRequest(name))

	// Current behaviour: the error is swallowed and no requeue is asked for, so
	// the object stays terminating until something else reconciles it.
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result.IsZero()).To(gomega.BeTrue())
	g.Expect(stub.callCount()).To(gomega.Equal(0))
	g.Expect(getVCDCluster(t, ctx, name).Finalizers).To(gomega.ContainElement(key.CleanerFinalizerName))
}

// vcdClusterIsGone reports whether the object left the api server.
func vcdClusterIsGone(ctx context.Context, name string) bool {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, &capvcd.VCDCluster{})

	return apierrors.IsNotFound(err)
}
