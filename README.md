# cluster-api-cleaner-cloud-director

A helper operator for CAPVCD to delete resources created by apps in workload clusters.

## CAPVCDClusterController

### Why?

<!-- `openstack-cloud-controller-manager` in workload cluster creates loadbalancers in OpenStack for services in the cluster. `openstack-cinder-csi` also creates some volumes in OpenStack for persistentvolumes in the cluster. When the worklaod cluster is deleted, `cluster-api-provider-openstack` doesn't clean these resources. This controller helps for clean-up of workload clusters. -->

### How does it work?

<!-- - It observes `OpenStackCluster` objects.
- It doesn't do anything in `reconcileNormal` other than adding finalizer.
- It respects `cluster.x-k8s.io/cluster-name` label in `OpenStackCluster` objects to get the actual cluster names.
- `clusterTag` is built as `giant_swarm_cluster_<management-cluster-name>_<workload_cluster-name>`.
- When an `OpenStackCluster` is deleted, it
  - cleans volumes ( whose metadata contains `cinder.csi.openstack.org/cluster: <clusterTag>` ) created by Cinder CSI 
  - cleans loadbalancers ( whose tags contain `kube_service_<clusterTag>.*` ) created by 
    openstack-cloud-controller-manager  -->

### Notes

This repo is heavilly inspired by the awesome [cluster-api-cleaner-openstack](https://github.com/giantswarm/cluster-api-cleaner-openstack).
