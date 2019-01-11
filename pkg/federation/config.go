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

package federation

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	configapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// MergeTargetClusterConfig merges the first cluster and the first AuthInfo from config2 into
// config1 using the given name. It then adds a context with the same name that points to the
// new cluster and authInfo.
func MergeTargetClusterConfig(name string, config1, config2 configapi.Config) *configapi.Config {
	for _, v := range config2.Clusters {
		config1.Clusters[name] = v
		break
	}
	for _, v := range config2.AuthInfos {
		config1.AuthInfos[name] = v
		break
	}
	context := &configapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}
	config1.Contexts[name] = context
	return &config1
}

// GenerateClientConfigFromRESTConfig generates a new kubeconfig using a given rest.Config.
// The rest.Config may come from in-cluster config (as in a pod) or an existing kubeconfig.
func GenerateClientConfigFromRESTConfig(name string, restConfig *rest.Config) *configapi.Config {
	cfg := &configapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       map[string]*configapi.Cluster{},
		AuthInfos:      map[string]*configapi.AuthInfo{},
		Contexts:       map[string]*configapi.Context{},
		CurrentContext: name,
	}

	cluster := &configapi.Cluster{
		Server:                   restConfig.Host,
		InsecureSkipTLSVerify:    restConfig.Insecure,
		CertificateAuthority:     restConfig.CAFile,
		CertificateAuthorityData: restConfig.CAData,
	}

	authInfo := &configapi.AuthInfo{
		ClientCertificate:     restConfig.CertFile,
		ClientCertificateData: restConfig.CertData,
		ClientKey:             restConfig.KeyFile,
		ClientKeyData:         restConfig.KeyData,
		Token:                 restConfig.BearerToken,
		Username:              restConfig.Username,
		Password:              restConfig.Password,
	}

	context := &configapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}

	cfg.Clusters[name] = cluster
	cfg.AuthInfos[name] = authInfo
	cfg.Contexts[name] = context

	return cfg
}

// GenerateCombinedKubeconfig takes in the bytes of a target cluster's kubeconfig and combines it with
// The hive cluster's kubeconfig to create a new kubeconfig that contains both.
func GenerateCombinedKubeconfig(targetKubeconfigBytes []byte, name string) ([]byte, error) {

	// Deserialize the target cluster's config
	targetCfg, err := clientcmd.Load(targetKubeconfigBytes)
	if err != nil {
		return nil, err
	}

	// Get the host cluster's rest config
	hostRESTCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	// Convert host's REST config to kubeconfig
	hostCfg := GenerateClientConfigFromRESTConfig("hive", hostRESTCfg)

	// Merge in the target kubeconfig
	mergedCfg := MergeTargetClusterConfig(name, *hostCfg, *targetCfg)

	// Serialize the merged kubeconfig
	return clientcmd.Write(*mergedCfg)
}
