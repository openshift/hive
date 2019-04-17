/*
Copyright 2018 The Kubernetes Authors.

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

package utils

import (
	"context"
	"fmt"

	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
)

// BuildClusterAPIClientFromKubeconfig will return a kubeclient using the provided kubeconfig
func BuildClusterAPIClientFromKubeconfig(kubeconfigData string) (client.Client, error) {
	config, err := clientcmd.Load([]byte(kubeconfigData))
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	scheme, err := machineapi.SchemeBuilder.Build()
	if err != nil {
		return nil, err
	}

	if err := openshiftapiv1.Install(scheme); err != nil {
		return nil, err
	}

	if err := routev1.Install(scheme); err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
	})
}

// BuildDynamicClientFromKubeconfig returns a dynamic client using the provided kubeconfig
func BuildDynamicClientFromKubeconfig(kubeconfigData string) (dynamic.Interface, error) {
	config, err := clientcmd.Load([]byte(kubeconfigData))
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// FixupEmptyClusterVersionFields will un-'nil' fields that would fail validation in the ClusterVersion.Status
func FixupEmptyClusterVersionFields(clusterVersionStatus *openshiftapiv1.ClusterVersionStatus) {

	// Fetching clusterVersion object can result in nil clusterVersion.Status.AvailableUpdates
	// Place an empty list if needed to satisfy the object validation.

	if clusterVersionStatus.AvailableUpdates == nil {
		clusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
	}
}

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

// GetKubeClient creates a new Kubernetes dynamic client.
func GetKubeClient(scheme *runtime.Scheme) (client.Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

// LoadSecretData loads a given secret key and returns it's data as a string.
func LoadSecretData(c client.Client, secretName, namespace, dataKey string) (string, error) {
	s := &kapi.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}
