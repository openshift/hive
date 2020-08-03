package resource

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func newFactory(clientGetter genericclioptions.RESTClientGetter) *namespacedFactory {
	cg := &namespacedClientGetter{RESTClientGetter: clientGetter}
	return &namespacedFactory{
		Factory:      cmdutil.NewFactory(cg),
		clientGetter: cg,
	}
}

type namespacedFactory struct {
	cmdutil.Factory
	clientGetter *namespacedClientGetter
}

func (f *namespacedFactory) getNamespace() string {
	return f.clientGetter.namespace
}

func (f *namespacedFactory) setNamespace(namespace string) {
	f.clientGetter.namespace = namespace
}

type namespacedClientGetter struct {
	genericclioptions.RESTClientGetter
	namespace string
}

func (g *namespacedClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return &namespacedClientConfig{
		clientConfig: g.RESTClientGetter.ToRawKubeConfigLoader(),
		namespace:    g.namespace,
	}
}

// This alias is needed since clientcmd.ClientConfig has a method named ClientConfig. Without the alias, the promoted
// method will conflict with the ClientConfig field name in namespacedClientConfig.
type clientConfig clientcmd.ClientConfig

type namespacedClientConfig struct {
	clientConfig
	namespace string
}

func (c *namespacedClientConfig) Namespace() (string, bool, error) {
	if c.namespace != "" {
		return c.namespace, true, nil
	}
	return c.clientConfig.Namespace()
}
