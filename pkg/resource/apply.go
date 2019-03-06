package resource

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	kcmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

const (
	defaultCacheDir = "/tmp"
	cacheDirEnvKey  = "CLI_CACHE_DIR"
)

// Info contains information obtained from a resource submitted to the Apply function
type Info struct {
	Name       string
	Namespace  string
	APIVersion string
	Kind       string
}

// Apply applies the given resource bytes to the target cluster specified by kubeconfig
func Apply(obj, kubeconfig []byte) (*Info, error) {

	cacheDir := defaultCacheDir
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		cacheDir = envCacheDir
	}
	// save obj to temporary file
	f, err := ioutil.TempFile(cacheDir, "kapply-")
	if err != nil {
		return nil, fmt.Errorf("cannot create temporary file: %v", err)
	}
	defer os.Remove(f.Name())
	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("cannot close temporary file %s: %v", f.Name(), err)
	}
	if err = ioutil.WriteFile(f.Name(), obj, 0660); err != nil {
		return nil, fmt.Errorf("cannot write to temporary file %s: %v", f.Name(), err)
	}
	factory, err := getFactory(cacheDir, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("could not create a factory: %v", err)
	}
	builder := factory.NewBuilder()
	infos, err := builder.Unstructured().Stream(bytes.NewBuffer(obj), "object").Flatten().Do().Infos()
	if err != nil {
		return nil, fmt.Errorf("could not get info from passed resource: %v", err)
	}
	if len(infos) != 1 {
		return nil, fmt.Errorf("unexpected number of resources found: %d", len(infos))
	}
	resourceInfo := &Info{
		Name:       infos[0].Name,
		Namespace:  infos[0].Namespace,
		Kind:       infos[0].ResourceMapping().GroupVersionKind.Kind,
		APIVersion: infos[0].ResourceMapping().GroupVersionKind.GroupVersion().String(),
	}
	ioStreams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	applyOptions, err := setupApplyCommand(factory, f.Name(), ioStreams)
	if err != nil {
		return nil, err
	}
	return resourceInfo, applyOptions.Run()
}

func getFactory(cacheDir string, kubeconfig []byte) (cmdutil.Factory, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, err
	}
	f := cmdutil.NewFactory(&remoteRESTClientGetter{clientConfig: clientConfig, cacheDir: cacheDir})
	return f, nil
}

func setupApplyCommand(f cmdutil.Factory, fileName string, ioStreams genericclioptions.IOStreams) (*kcmd.ApplyOptions, error) {
	o := kcmd.NewApplyOptions(ioStreams)
	o.ToPrinter = func(string) (printers.ResourcePrinter, error) { return o.PrintFlags.ToPrinter() }
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return nil, err
	}
	o.DeleteOptions = o.DeleteFlags.ToOptions(dynamicClient, o.IOStreams)
	o.OpenAPISchema, _ = f.OpenAPISchema()
	o.Validator, err = f.Validator(false)
	o.Builder = f.NewBuilder()
	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		return nil, err
	}

	o.DynamicClient = dynamicClient
	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, err
	}

	o.DeleteOptions.FilenameOptions.Filenames = []string{fileName}

	return o, nil
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
