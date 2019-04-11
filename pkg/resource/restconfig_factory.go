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

func (r *Helper) getRESTConfigFactory(namespace string) (cmdutil.Factory, error) {
	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from REST client config and cache directory")
	f := cmdutil.NewFactory(&restConfigClientGetter{restConfig: r.restConfig, cacheDir: r.cacheDir, namespace: namespace})
	return f, nil
}

type restConfigClientGetter struct {
	restConfig *rest.Config
	cacheDir   string
	namespace  string
}

// ToRESTConfig returns restconfig
func (r *restConfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

// ToDiscoveryClient returns discovery client
func (r *restConfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config := rest.CopyConfig(r.restConfig)
	return getDiscoveryClient(config, r.cacheDir)
}

// ToRESTMapper returns a restmapper
func (r *restConfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *restConfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cfg := GenerateClientConfigFromRESTConfig("default", r.restConfig)
	overrides := &clientcmd.ConfigOverrides{}
	if len(r.namespace) > 0 {
		overrides.Context.Namespace = r.namespace
	}
	return clientcmd.NewNonInteractiveClientConfig(*cfg, "", overrides, nil)
}
