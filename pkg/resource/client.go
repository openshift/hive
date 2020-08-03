package resource

import (
	"io/ioutil"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"
	configapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	tokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

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

	if restConfig.WrapTransport != nil && len(restConfig.BearerToken) == 0 {
		token, err := ioutil.ReadFile(tokenFile)
		if err != nil {
			log.WithError(err).Warning("empty bearer token and cannot read token file")
		} else {
			authInfo.Token = string(token)
		}
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
