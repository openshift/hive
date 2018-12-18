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
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capiv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	openshiftapiv1 "github.com/openshift/api/config/v1"
)

// BuildClusterAPIClientFromKubeconfig will return a kubeclient usin the provided kubeconfig
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

	scheme, err := capiv1.SchemeBuilder.Build()
	if err != nil {
		return nil, err
	}
	return client.New(cfg, client.Options{
		Scheme: scheme,
	})
}

// FixupEmptyClusterVersionFields will un-'nil' fields that would fail validation in the ClusterVersion.Status
func FixupEmptyClusterVersionFields(clusterVersionStatus *openshiftapiv1.ClusterVersionStatus) {

	// Fetching clusterVersion object can results in nil clusterVersion.Status.AvailableUpdates
	// and clusterVersion.Status.Conditions.
	// Place an empty list if needed to satisfy the object validation.

	if clusterVersionStatus.AvailableUpdates == nil {
		clusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
	}

	if clusterVersionStatus.Conditions == nil {
		clusterVersionStatus.Conditions = []openshiftapiv1.ClusterOperatorStatusCondition{}
	}
}
