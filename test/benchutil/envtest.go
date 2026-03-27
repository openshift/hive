package benchutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gobuffalo/flect"
	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	log "github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivescheme "github.com/openshift/hive/pkg/util/scheme"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	testEnv        *envtest.Environment
	testRESTConfig *rest.Config
	seedClient     client.Client // initialized in envtestOnce; see newClient in client.go
	envtestOnce    sync.Once
)

// EnsureEnvTest lazily starts a shared envtest environment. Torn down via StopEnvTest.
func EnsureEnvTest(b *testing.B) *rest.Config {
	b.Helper()
	envtestOnce.Do(func() {
		// Verify KUBEBUILDER_ASSETS is set before starting envtest
		if os.Getenv("KUBEBUILDER_ASSETS") == "" {
			fmt.Fprintf(os.Stderr, `
	ERROR: KUBEBUILDER_ASSETS environment variable not set

	Benchmarks require envtest binaries (kube-apiserver, etcd).

	To set up:
	  $ make setup-envtest

	Then export the path shown, or run:
	  $ export KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh | tail -1 | cut -d'=' -f2 | tr -d '"')"

	See test/benchmark/README.md for details.

`)
			os.Exit(1)
		}

		logf.SetLogger(logr.New(logr.Discard().GetSink()))
		log.SetLevel(log.ErrorLevel)
		if err := hivev1.AddToScheme(scheme.Scheme); err != nil {
			fmt.Fprintf(os.Stderr, "failed to add hive scheme: %v\n", err)
			os.Exit(1)
		}
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: hiveCRDPaths(),
			CRDs:              openshiftCRDs(),
		}
		var err error
		testRESTConfig, err = testEnv.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start test environment: %v\n", err)
			os.Exit(1)
		}
		seedClient, err = newClient(testRESTConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create seed client: %v\n", err)
			os.Exit(1)
		}
	})
	return testRESTConfig
}

// StopEnvTest tears down the shared envtest environment.
func StopEnvTest() {
	if testEnv != nil {
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop test environment: %v\n", err)
		}
	}
}

// hiveCRDPaths returns the path to Hive's CRD manifests.
func hiveCRDPaths() []string {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/openshift/hive").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate module root: %v\n", err)
		os.Exit(1)
	}
	root := strings.TrimSpace(string(out))
	crdDir := filepath.Join(root, "config", "crds")
	if _, err := os.Stat(crdDir); err != nil {
		fmt.Fprintf(os.Stderr, "CRD directory not found at %s: %v\n", crdDir, err)
		os.Exit(1)
	}
	return []string{crdDir}
}

// openshiftCRDs returns minimal CRDs for OpenShift platform types.
func openshiftCRDs() []*apiextensionsv1.CustomResourceDefinition {
	return []*apiextensionsv1.CustomResourceDefinition{
		crdForType(&configv1.ClusterVersion{}, apiextensionsv1.ClusterScoped),
		crdForType(&configv1.ClusterOperator{}, apiextensionsv1.ClusterScoped),
		crdForType(&machinev1beta1.Machine{}, apiextensionsv1.NamespaceScoped),
		crdForType(&machinev1beta1.MachineSet{}, apiextensionsv1.NamespaceScoped),
	}
}

// crdForType builds a minimal CRD from a typed empty object.
func crdForType(obj runtime.Object, scope apiextensionsv1.ResourceScope) *apiextensionsv1.CustomResourceDefinition {
	gvks, _, err := hivescheme.GetScheme().ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		panic(fmt.Sprintf("crdForType: type not registered in scheme: %T", obj))
	}
	gvk := gvks[0]
	plural := strings.ToLower(flect.Pluralize(gvk.Kind))

	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + gvk.Group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gvk.Group,
			Scope: scope,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: strings.ToLower(gvk.Kind),
				Kind:     gvk.Kind,
				ListKind: gvk.Kind + "List",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    gvk.Version,
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:                   "object",
						XPreserveUnknownFields: boolPtr(true),
					},
				},
			}},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
