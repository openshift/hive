/*
Copyright 2019 The Kubernetes Authors.

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

package resource

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func (r *Helper) getKubeconfigFactory(namespace string) (cmdutil.Factory, error) {
	r.logger.Debug("loading kubeconfig from byte array")
	config, err := clientcmd.Load(r.kubeconfig)
	if err != nil {
		r.logger.WithError(err).Error("an error occurred loading the kubeconfig")
		return nil, err
	}
	overrides := &clientcmd.ConfigOverrides{}
	if len(namespace) > 0 {
		r.logger.WithField("namespace", namespace).Debug("specifying override namespace on clientconfig")
		overrides.Context.Namespace = namespace
	}
	r.logger.Debug("creating client config from kubeconfig")
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*config, "", overrides, nil)

	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from client config and cache directory")
	f := cmdutil.NewFactory(&kubeconfigClientGetter{clientConfig: clientConfig, cacheDir: r.cacheDir})
	return f, nil
}

type kubeconfigClientGetter struct {
	clientConfig clientcmd.ClientConfig
	cacheDir     string
}

// ToRESTConfig returns restconfig
func (r *kubeconfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.ToRawKubeConfigLoader().ClientConfig()
}

// ToDiscoveryClient returns discovery client
func (r *kubeconfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return getDiscoveryClient(config, r.cacheDir)
}

// ToRESTMapper returns a restmapper
func (r *kubeconfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *kubeconfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.clientConfig
}
