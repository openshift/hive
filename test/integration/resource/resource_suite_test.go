package resource

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/openshift/hive/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config
var c client.Client
var kubeconfig []byte

const (
	kubeconfigTemplate = `
apiVersion: v1
clusters:
- cluster:
    server: %s
  name: cluster1
contexts:
- context:
    cluster: cluster1
  name: context1
current-context: context1
kind: Config
preferences: {}
`
)

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	}
	apis.AddToScheme(scheme.Scheme)
	corev1.AddToScheme(scheme.Scheme)

	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		log.Fatal(err)
	}

	kubeconfig = []byte(fmt.Sprintf(kubeconfigTemplate, cfg.Host))

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
