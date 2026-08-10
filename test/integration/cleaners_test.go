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
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/cleaner"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/test/integration/vcdfake"
)

// newCleanerFixture starts a fake VCD, creates the VCDCluster that points at
// it, and logs in. infraId is the marker the cleaners match resources on.
func newCleanerFixture(t *testing.T, cfg vcdfake.Config) (*vcdfake.Server, *vcdsdk.Client, *capvcd.VCDCluster) {
	t.Helper()
	g := gomega.NewWithT(t)
	ctx := context.Background()
	name := uniqueName(t)

	// The volume cleaner looks vms up in the vApp named after the cluster.
	cfg.VAppName = name

	server := newVCDServer(t, cfg)

	vcdCluster := createVCDCluster(t, ctx, newVCDCluster(name, server.URL(), nil), infraIdFor(t))

	client, err := vcd.GetVCDClient(ctx, k8sClient, vcdCluster, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	return server, client, vcdCluster
}

// infraIdFor is the cluster marker VCD resource names carry.
func infraIdFor(t *testing.T) string {
	t.Helper()

	return "infra-" + uniqueName(t)
}

func TestVolumeCleaner(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	infraId := infraIdFor(t)

	// The matching disks are spread over two query pages, and one of them is
	// still attached to a node.
	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		DiskPages: [][]vcdfake.Disk{
			{
				{ID: "disk-1", Name: "pvc-one", Description: infraId, AttachedVM: "node-0"},
				{ID: "disk-2", Name: "pvc-other", Description: "other-cluster"},
			},
			{
				{ID: "disk-3", Name: "pvc-two", Description: infraId},
			},
		},
	})

	requeue, err := cleaner.NewVolumeCleaner(k8sClient).Clean(ctx, logr.Discard(), client, vcdCluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(requeue).To(gomega.BeFalse())

	// The attached disk is released before it is removed.
	g.Expect(server.DetachedDisks()).To(gomega.Equal([]string{"pvc-one"}))

	// Both pages are cleaned, and a disk belonging to another cluster is left
	// alone.
	g.Expect(server.DeletedDisks()).To(gomega.ConsistOf("pvc-one", "pvc-two"))

	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}

func TestDNATCleaner(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	infraId := infraIdFor(t)

	// One rule per page, so the cursor paging really runs.
	//
	// The two matching rules are next to each other on purpose. A cursor is an
	// offset into the live list, so deleting a rule while paging shifts every
	// later rule up by one and the next page skips one. Ordering them this way
	// makes that skip drop a rule the cleaner had to delete.
	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		NatRulePageSize: 1,
		NatRules: []vcdfake.Resource{
			{ID: "nat-1", Name: "dnat-" + infraId + "-a"},
			{ID: "nat-2", Name: "dnat-" + infraId + "-b"},
			{ID: "nat-3", Name: "dnat-other-cluster"},
		},
	})

	requeue, err := cleaner.NewDNATCleaner(k8sClient).Clean(ctx, logr.Discard(), client, vcdCluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(requeue).To(gomega.BeFalse())

	g.Expect(server.DeletedNatRules()).To(gomega.ConsistOf("dnat-"+infraId+"-a", "dnat-"+infraId+"-b"))

	// Every page was read, so the cursor handling really ran.
	g.Expect(server.Requests()).To(gomega.ContainElement(gomega.ContainSubstring("cursor=3")))

	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}

func TestVirtualServiceCleaner(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	infraId := infraIdFor(t)

	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		VirtualServices: []vcdfake.Resource{
			{ID: "vs-1", Name: "svc-" + infraId + "-a"},
			{ID: "vs-2", Name: "svc-other-cluster"},
			{ID: "vs-3", Name: "svc-" + infraId + "-b"},
		},
	})

	requeue, err := cleaner.NewVirtualServiceCleaner(k8sClient).Clean(ctx, logr.Discard(), client, vcdCluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(requeue).To(gomega.BeFalse())

	g.Expect(server.DeletedVirtualServices()).To(gomega.ConsistOf("svc-"+infraId+"-a", "svc-"+infraId+"-b"))
	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}

func TestLBPoolCleaner(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	infraId := infraIdFor(t)

	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		Pools: []vcdfake.Resource{
			{ID: "pool-1", Name: "pool-" + infraId + "-a"},
			{ID: "pool-2", Name: "pool-other-cluster"},
			{ID: "pool-3", Name: "pool-" + infraId + "-b"},
		},
	})

	requeue, err := cleaner.NewLBPoolCleaner(k8sClient).Clean(ctx, logr.Discard(), client, vcdCluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(requeue).To(gomega.BeFalse())

	g.Expect(server.DeletedPools()).To(gomega.ConsistOf("pool-"+infraId+"-a", "pool-"+infraId+"-b"))
	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}

func TestAppPortProfileCleaner(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()
	infraId := infraIdFor(t)

	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		AppPortProfiles: []vcdfake.Resource{
			{ID: "app-1", Name: "appPort-" + infraId + "-a"},
			{ID: "app-2", Name: "appPort-other-cluster"},
			{ID: "app-3", Name: "appPort-" + infraId + "-b"},
		},
	})

	requeue, err := cleaner.NewAppPortProfileCleaner(k8sClient).Clean(ctx, logr.Discard(), client, vcdCluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(requeue).To(gomega.BeFalse())

	g.Expect(server.DeletedAppPortProfiles()).To(gomega.ConsistOf("appPort-"+infraId+"-a", "appPort-"+infraId+"-b"))

	// The listing is limited to the tenant scope.
	g.Expect(server.Requests()).To(gomega.ContainElement(gomega.ContainSubstring("scope%3D%3DTENANT")))

	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}

// TestCleanersLeaveOtherClustersAlone runs every cleaner against resources that
// belong to a different cluster.
func TestCleanersLeaveOtherClustersAlone(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	server, client, vcdCluster := newCleanerFixture(t, vcdfake.Config{
		DiskPages:       [][]vcdfake.Disk{{{ID: "disk-1", Name: "pvc-other", Description: "other-cluster"}}},
		NatRules:        []vcdfake.Resource{{ID: "nat-1", Name: "dnat-other-cluster"}},
		VirtualServices: []vcdfake.Resource{{ID: "vs-1", Name: "svc-other-cluster"}},
		Pools:           []vcdfake.Resource{{ID: "pool-1", Name: "pool-other-cluster"}},
		AppPortProfiles: []vcdfake.Resource{{ID: "app-1", Name: "appPort-other-cluster"}},
	})

	cleaners := []cleaner.Cleaner{
		cleaner.NewVolumeCleaner(k8sClient),
		cleaner.NewVirtualServiceCleaner(k8sClient),
		cleaner.NewLBPoolCleaner(k8sClient),
		cleaner.NewDNATCleaner(k8sClient),
		cleaner.NewAppPortProfileCleaner(k8sClient),
	}

	for _, c := range cleaners {
		requeue, err := c.Clean(ctx, logr.Discard(), client, vcdCluster)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(requeue).To(gomega.BeFalse())
	}

	g.Expect(server.DeletedDisks()).To(gomega.BeEmpty())
	g.Expect(server.DeletedNatRules()).To(gomega.BeEmpty())
	g.Expect(server.DeletedVirtualServices()).To(gomega.BeEmpty())
	g.Expect(server.DeletedPools()).To(gomega.BeEmpty())
	g.Expect(server.DeletedAppPortProfiles()).To(gomega.BeEmpty())
	g.Expect(server.Unhandled()).To(gomega.BeEmpty())
}
