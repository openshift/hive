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
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func (r *Helper) getKubeconfigFactory(kubeconfig []byte, namespace string) (cmdutil.Factory, error) {
	r.logger.Debug("loading kubeconfig from byte array")
	config, err := clientcmd.Load(kubeconfig)
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
	f := cmdutil.NewFactory(&remoteRESTClientGetter{clientConfig: clientConfig, cacheDir: r.cacheDir})
	return f, nil
}

type remoteRESTClientGetter struct {
	clientConfig clientcmd.ClientConfig
	cacheDir     string
}

// ToRESTConfig returns restconfig
func (r *remoteRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.ToRawKubeConfigLoader().ClientConfig()
}

// ToDiscoveryClient returns discovery client
func (r *remoteRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	config.Burst = 100
	httpCacheDir := filepath.Join(r.cacheDir, ".kube", "http-cache")
	discoveryCacheDir := computeDiscoverCacheDir(filepath.Join(r.cacheDir, ".kube", "cache", "discovery"), config.Host)
	return discovery.NewCachedDiscoveryClientForConfig(config, discoveryCacheDir, httpCacheDir, time.Duration(10*time.Minute))
}

// ToRESTMapper returns a restmapper
func (r *remoteRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *remoteRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.clientConfig
}

// overlyCautiousIllegalFileCharacters matches characters that *might* not be supported.  Windows is really restrictive, so this is really restrictive
var overlyCautiousIllegalFileCharacters = regexp.MustCompile(`[^(\w/\.)]`)

// computeDiscoverCacheDir takes the parentDir and the host and comes up with a "usually non-colliding" name.
func computeDiscoverCacheDir(parentDir, host string) string {
	// strip the optional scheme from host if its there:
	schemelessHost := strings.Replace(strings.Replace(host, "https://", "", 1), "http://", "", 1)
	// now do a simple collapse of non-AZ09 characters.  Collisions are possible but unlikely.  Even if we do collide the problem is short lived
	safeHost := overlyCautiousIllegalFileCharacters.ReplaceAllString(schemelessHost, "_")
	return filepath.Join(parentDir, safeHost)
}
