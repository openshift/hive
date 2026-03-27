package benchutil

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"
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

// CopyAndSetNamespace deep-copies a client.Object and sets its namespace.
func CopyAndSetNamespace(obj client.Object, ns string) client.Object {
	if obj == nil {
		return nil
	}
	runtimeCopied := obj.DeepCopyObject()
	copied, ok := runtimeCopied.(client.Object)
	if !ok {
		panic(fmt.Sprintf("CopyAndSetNamespace: DeepCopyObject of %T did not return a client.Object", obj))
	}
	copied.SetNamespace(ns)
	return copied
}

// MustSerialize serializes a runtime.Object to JSON. Panics on error.
func MustSerialize(obj runtime.Object) []byte {
	out, err := resource.Serialize(obj, scheme.Scheme)
	if err != nil {
		panic(fmt.Sprintf("MustSerialize: %v", err))
	}
	return out
}
