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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/cleaner"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/key"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/vcd"
)

// VCDClusterReconciler reconciles a vcdCluster object
type VCDClusterReconciler struct {
	client.Client
	Log logr.Logger

	ManagementCluster string
	Cleaners          []cleaner.Cleaner
}

// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/status,verbs=get;update;patch

func (r *VCDClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("vcdcluster", req.NamespacedName)
	log.V(1).Info("Reconciling")

	var infraCluster capvcd.VCDCluster
	err := r.Get(ctx, req.NamespacedName, &infraCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Fetch the owner cluster.
	coreCluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}
	if coreCluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{}, nil
	}

	log = log.WithValues("cluster", coreCluster.Name)

	// Return early if the core or infrastructure cluster is paused.
	if annotations.IsPaused(coreCluster, &infraCluster) {
		log.Info("infrastructure or core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Handle deleted clusters
	if !infraCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, &infraCluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, log, &infraCluster)
}

func (r *VCDClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capvcd.VCDCluster{}).
		Complete(r)
}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, log logr.Logger, vcdCluster *capvcd.VCDCluster) (reconcile.Result, error) {
	// If the vcdCluster doesn't have the finalizer, add it.
	if !controllerutil.ContainsFinalizer(vcdCluster, key.CleanerFinalizerName) {
		controllerutil.AddFinalizer(vcdCluster, key.CleanerFinalizerName)
		// Register the finalizer immediately to avoid orphaning VCD resources on delete
		if err := r.Update(ctx, vcdCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
	}

	// Cleaner doesn't do anything for normal
	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileDelete(ctx context.Context, log logr.Logger, vcdCluster *capvcd.VCDCluster) (reconcile.Result, error) {
	if !controllerutil.ContainsFinalizer(vcdCluster, key.CleanerFinalizerName) {
		// no-op in case the finalizer is not there (it could have been deleted manually)
		return ctrl.Result{}, nil
	}

	clusterName, ok := vcdCluster.Labels[key.CapiClusterLabelKey]
	if !ok {
		log.V(1).Info("VCDcluster doesn't have necessary label",
			"expectedLabelKey", key.CapiClusterLabelKey,
			"existingLabels", vcdCluster.Labels)
		return ctrl.Result{}, nil
	}
	if len(vcdCluster.Status.InfraId) == 0 {
		e := fmt.Errorf(".status.infraId is not populated on the cluster: %s", vcdCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, microerror.Mask(e)
	}

	vcdClient, err := vcd.GetVCDClient(ctx, r.Client, vcdCluster, log)
	if err != nil {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Cleaning VCD resources belonging to cluster", "cluster", clusterName)
	requeueForDeletion := false
	for _, c := range r.Cleaners {
		requeue, err := c.Clean(ctx, log, vcdClient, vcdCluster)
		if err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
		requeueForDeletion = requeueForDeletion || requeue
	}

	if requeueForDeletion {
		log.V(1).Info("There is an ongoing clean-up process. Adding cluster into queue again")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
	}

	log.Info("Clean-up is done. Removing finalizer")
	// vcdCluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(vcdCluster, key.CleanerFinalizerName)
	// Finally remove the finalizer
	if err := r.Update(ctx, vcdCluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{}, nil
}
