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

package util

import (
	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"

	"k8s.io/client-go/rest"
	configapi "k8s.io/client-go/tools/clientcmd/api"
)

// ApplyAsset loads a path from our bindata assets and applies it to the cluster.
func ApplyAsset(h *resource.Helper, assetPath string, hLog log.FieldLogger) error {
	assetLog := hLog.WithField("asset", assetPath)
	assetLog.Debug("reading asset")
	asset := assets.MustAsset(assetPath)
	assetLog.Debug("applying asset")
	err := h.Apply(asset)
	if err != nil {
		assetLog.WithError(err).Error("error applying hive reader role")
		return err
	}
	assetLog.Info("asset applied successfully")
	return nil
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
