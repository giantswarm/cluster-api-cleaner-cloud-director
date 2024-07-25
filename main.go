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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/cluster-api-cleaner-cloud-director/controllers"
	"github.com/giantswarm/cluster-api-cleaner-cloud-director/pkg/cleaner"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = capvcd.AddToScheme(scheme)
	_ = capi.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	err := mainE(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\nTo increase verbosity, re-run with --level=debug\n", microerror.Pretty(err, true))
		os.Exit(2)
	}
}

func mainE(ctx context.Context) error {
	var (
		enableLeaderElection bool
		managementCluster    string
		metricsAddr          string
		logLevel             int
	)

	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")

	flag.StringVar(&managementCluster, "management-cluster", "", "Name of the management cluster.")

	flag.IntVar(&logLevel, "v", 0, "Number for the log level verbosity")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.Level(zapcore.Level(-logLevel))))

	config, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "cluster-api-cleaner-cloud-director.giantswarm.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	cleaners := []cleaner.Cleaner{
		cleaner.NewVolumeCleaner(mgr.GetClient()),
		cleaner.NewVirtualServiceCleaner(mgr.GetClient()),
		cleaner.NewLBPoolCleaner(mgr.GetClient()),
		cleaner.NewDNATCleaner(mgr.GetClient()),
		cleaner.NewAppPortProfileCleaner(mgr.GetClient()),
	}

	if err = (&controllers.VCDClusterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("VCDCluster"),

		ManagementCluster: managementCluster,
		Cleaners:          cleaners,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VCDCluster")
		return err
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}
