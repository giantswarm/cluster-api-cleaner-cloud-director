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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	capvcd "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testScheme *runtime.Scheme
	k8sClient  client.Client
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	testScheme = runtime.NewScheme()
	// Register exactly the api versions that main.go registers, and no more.
	// envtest injects a conversion webhook when it finds more than one version of
	// a convertible Kind in the scheme. Nothing serves that webhook, so adding
	// capvcd api/v1beta2 here breaks every VCDCluster test.
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(capvcd.AddToScheme(testScheme))
	utilruntime.Must(capi.AddToScheme(testScheme))

	capvcdDir, err := moduleDir("github.com/vmware/cluster-api-provider-cloud-director")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	capiDir, err := moduleDir("sigs.k8s.io/cluster-api")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}

	testEnv := &envtest.Environment{
		Scheme: testScheme,
		CRDDirectoryPaths: []string{
			filepath.Join(capvcdDir, "config", "crd", "bases", "infrastructure.cluster.x-k8s.io_vcdclusters.yaml"),
			filepath.Join(capiDir, "config", "crd", "bases", "cluster.x-k8s.io_clusters.yaml"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting envtest: %s\n", err)
		fmt.Fprintf(os.Stderr, "Run the tests with `make integration-test` so KUBEBUILDER_ASSETS is set.\n")
		return 1
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping envtest: %s\n", err)
		}
	}()

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building client: %s\n", err)
		return 1
	}

	return m.Run()
}

// moduleDir returns the local directory of a dependency. It resolves the module
// through `go list`, so it honours the `replace` directives in go.mod and no CRD
// yaml needs to be copied into this repository.
func moduleDir(modulePath string) (string, error) {
	// modulePath is always a constant from this package, never user input.
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", modulePath).Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("unable to resolve module %s: %w", modulePath, err)
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("module %s has no local directory, run `go mod download`", modulePath)
	}

	return dir, nil
}
