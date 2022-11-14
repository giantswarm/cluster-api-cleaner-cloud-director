package key

const (
	ClusterTagPrefix     = "giant_swarm_cluster"
	CapiClusterLabelKey  = "cluster.x-k8s.io/cluster-name"
	CleanerFinalizerName = "cluster-api-cleaner-cloud-director.finalizers.giantswarm.io"

	LoadBalancerProvisioningStatusActive = "ACTIVE"
	LoadBalancerProvisioningStatusError  = "ERROR"
	VolumeStatusDeleting                 = "deleting"
)
