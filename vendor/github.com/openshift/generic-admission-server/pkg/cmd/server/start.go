package server

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	admissionv1 "k8s.io/api/admission/v1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/features"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift/generic-admission-server/pkg/apiserver"
	"github.com/openshift/generic-admission-server/pkg/registry/admissionreview/generated"
)

const defaultEtcdPathPrefix = "/registry/online.openshift.io"

type AdmissionServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions

	AdmissionHooks []apiserver.AdmissionHook

	StdOut io.Writer
	StdErr io.Writer
}

func NewAdmissionServerOptions(out, errOut io.Writer, admissionHooks ...apiserver.AdmissionHook) *AdmissionServerOptions {
	o := &AdmissionServerOptions{
		// TODO we will nil out the etcd storage options.  This requires a later level of k8s.io/apiserver
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(admissionv1.SchemeGroupVersion, admissionv1beta1.SchemeGroupVersion),
		),

		AdmissionHooks: admissionHooks,

		StdOut: out,
		StdErr: errOut,
	}
	o.RecommendedOptions.Etcd = nil
	o.RecommendedOptions.Admission = nil

	// we can also optimize the authz options.  We know that system:masters should always be authorized for actions and the
	// delegating authorizer now allows this.
	o.RecommendedOptions.Authorization = o.RecommendedOptions.Authorization.
		WithAlwaysAllowPaths("/healthz", "/readyz", "/livez"). // this allows the kubelet to always get health and readiness without causing an access check
		WithAlwaysAllowGroups("system:masters")                // in a kube cluster, system:masters can take any action, so there is no need to ask for an authz check

	return o
}

// NewCommandStartMaster provides a CLI handler for 'start master' command
func NewCommandStartAdmissionServer(out, errOut io.Writer, stopCh <-chan struct{}, admissionHooks ...apiserver.AdmissionHook) *cobra.Command {
	o := NewAdmissionServerOptions(out, errOut, admissionHooks...)

	cmd := &cobra.Command{
		Short: "Launch a namespace reservation API server",
		Long:  "Launch a namespace reservation API server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunAdmissionServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.AddFlags(flags)

	return cmd
}

func (o *AdmissionServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
	// first set the UnauthenticatedHTTP2DOSMitigation feature to true by default
	if err := feature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.UnauthenticatedHTTP2DOSMitigation): true,
	}); err != nil {
		panic(err)
	}
	// an operator should be able to override the feature gate if it is passed from CLI
	feature.DefaultMutableFeatureGate.AddFlag(fs)

	// it is not possible to modify imported feature gates (Default field)
	// make sure the feature-gates help represents reality
	fs.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "feature-gates" {
			fgUsage := strings.Split(flag.Usage, "\n")
			for i, fg := range fgUsage {
				if strings.Contains(fg, string(features.UnauthenticatedHTTP2DOSMitigation)+"=") {
					fgUsage[i] = strings.Replace(fg, "default=false", "default=true", 1)
					break
				}
			}
			flag.Usage = strings.Join(fgUsage, "\n")
		}
	})
}

func (o AdmissionServerOptions) Validate(args []string) error {
	return nil
}

func (o *AdmissionServerOptions) Complete() error {
	return nil
}

func (o AdmissionServerOptions) Config() (*apiserver.Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(generated.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(apiserver.Scheme))
	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	kubeconfigFile := o.RecommendedOptions.CoreAPI.CoreAPIKubeconfigPath
	restConfig, err := getClientConfig(kubeconfigFile)
	if err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig: apiserver.ExtraConfig{
			AdmissionHooks: o.AdmissionHooks,
		},
		RestConfig: restConfig,
	}
	return config, nil
}

func (o AdmissionServerOptions) RunAdmissionServer(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}
	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

func getClientConfig(kubeconfigFile string) (*rest.Config, error) {
	var kubeconfig *rest.Config
	var err error
	if len(kubeconfigFile) > 0 {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigFile}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		kubeconfig, err = loader.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig at %q: %v", kubeconfigFile, err)
		}
	} else {
		kubeconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}
	return kubeconfig, nil
}
