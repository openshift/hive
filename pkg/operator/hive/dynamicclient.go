package hive

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// This file exists to provide dynamicClient methods for things that we would like to be able to
// get via a regular controller-runtime client, but doing so seems to cause cache problems.

// TODO: This sucks
func (r *ReconcileHiveConfig) clientFor(blankObj runtime.Object, namespace string) dynamic.ResourceInterface {
	dc := r.dynamicClient
	var c dynamic.NamespaceableResourceInterface
	switch blankObj.(type) {
	case *hivev1.HiveConfigList, *hivev1.HiveConfig:
		c = dc.Resource(hivev1.SchemeGroupVersion.WithResource("hiveconfigs"))
	case *apiextv1.CustomResourceDefinitionList, *apiextv1.CustomResourceDefinition:
		c = dc.Resource(apiextv1.SchemeGroupVersion.WithResource("customresourcedefinitions"))
	case *appsv1.StatefulSetList, *appsv1.StatefulSet:
		c = dc.Resource(appsv1.SchemeGroupVersion.WithResource("statefulsets"))
	case *corev1.ConfigMapList, *corev1.ConfigMap:
		c = dc.Resource(corev1.SchemeGroupVersion.WithResource("configmaps"))
	case *corev1.NamespaceList, *corev1.Namespace:
		c = dc.Resource(corev1.SchemeGroupVersion.WithResource("namespaces"))
	case *configv1.ProxyList, *configv1.Proxy:
		c = dc.Resource(configv1.SchemeGroupVersion.WithResource("proxies"))
	case *configv1.APIServerList, *configv1.APIServer:
		c = dc.Resource(configv1.SchemeGroupVersion.WithResource("apiservers"))
	case *corev1.SecretList, *corev1.Secret:
		c = dc.Resource(corev1.SchemeGroupVersion.WithResource("secrets"))
	case *corev1.ServiceAccountList, *corev1.ServiceAccount:
		c = dc.Resource(corev1.SchemeGroupVersion.WithResource("serviceaccounts"))
	}
	if c == nil {
		panic(fmt.Sprintf("You forgot to make a case for clients of type %T", blankObj))
	}
	if namespace == "" {
		return c
	}
	return c.Namespace(namespace)
}

func (r *ReconcileHiveConfig) List(ctx context.Context, obj runtime.Object, namespace string, listOpts metav1.ListOptions) error {
	client := r.clientFor(obj, namespace)

	usList, err := client.List(ctx, listOpts)
	if err != nil {
		errors.Wrapf(err, "error listing unstructured %T", obj)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(usList.UnstructuredContent(), obj); err != nil {
		return errors.Wrapf(err, "error converting unstructured to %T", obj)
	}
	return nil
}

func (r *ReconcileHiveConfig) Get(ctx context.Context, nsName types.NamespacedName, obj runtime.Object) error {
	client := r.clientFor(obj, nsName.Namespace)
	usObj, err := client.Get(ctx, nsName.Name, metav1.GetOptions{})
	if err != nil {
		// Caller needs to be able to do things like apierrors.IsNotFound() on this
		return err
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(usObj.UnstructuredContent(), obj); err != nil {
		return errors.Wrapf(err, "error converting unstructured to %T for %v", obj, nsName)
	}
	return nil
}
