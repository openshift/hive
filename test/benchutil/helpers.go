package benchutil

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"
	hivescheme "github.com/openshift/hive/pkg/util/scheme"
)

// BenchHelper creates a resource.Helper with suppressed logging.
func BenchHelper(b *testing.B, cfg *rest.Config, controllerName string) resource.Helper {
	b.Helper()
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)
	h, err := resource.NewHelper(logger, resource.FromRESTConfig(cfg), resource.WithControllerName(hivev1.ControllerName(controllerName)))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}
	return h
}

// CopyAndSetNamespace deep-copies each object, sets its namespace, and returns
// the copies. Nil elements are skipped. The input objects are never modified,
// so templates can safely be shared across benchmarks.
func CopyAndSetNamespace(ns string, objs ...client.Object) []client.Object {
	copied := make([]client.Object, 0, len(objs))
	for _, obj := range objs {
		if obj == nil {
			continue
		}
		runtimeCopied := obj.DeepCopyObject()
		c, ok := runtimeCopied.(client.Object)
		if !ok {
			panic(fmt.Sprintf("CopyAndSetNamespace: DeepCopyObject of %T did not return a client.Object", obj))
		}
		c.SetNamespace(ns)
		copied = append(copied, c)
	}
	return copied
}

// MustSerialize serializes a runtime.Object to JSON using the Hive scheme
// (which covers everything the benchmarks apply, including OpenShift types).
// Panics on error.
func MustSerialize(obj runtime.Object) []byte {
	out, err := resource.Serialize(obj, hivescheme.GetScheme())
	if err != nil {
		panic(fmt.Sprintf("MustSerialize: %v", err))
	}
	return out
}
