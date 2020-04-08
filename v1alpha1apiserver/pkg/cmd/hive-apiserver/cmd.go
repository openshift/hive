package hiveapiserver

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	kcmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/util/templates"

	"github.com/openshift/library-go/pkg/serviceability"
)

// HiveAPIServer is a Hive aggregated API server
type HiveAPIServer struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	Output             io.Writer
}

var longDescription = templates.LongDesc(`
	Start an apiserver that proxies Hive v1alpha1 resources to Hive v1 resources`)

// NewHiveAPIServerCommand creates a cobra command for starting a Hive aggregated API server.
func NewHiveAPIServerCommand(name string, out, errout io.Writer, stopCh <-chan struct{}) *cobra.Command {
	options := &HiveAPIServer{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			"/registry/hive.openshift.io",
			serializer.NewCodecFactory(runtime.NewScheme()).LegacyCodec(),
			&genericoptions.ProcessInfo{
				Name:      "hive-apiserver",
				Namespace: "hive",
			},
		),
		Output: out,
	}

	cmd := &cobra.Command{
		Use:   name,
		Short: "Launch Hive v1alpha1 apiserver",
		Long:  longDescription,
		Run: func(c *cobra.Command, args []string) {
			kcmdutil.CheckErr(options.Validate())

			serviceability.StartProfiler()

			if err := options.RunAPIServer(stopCh); err != nil {
				if kerrors.IsInvalid(err) {
					if details := err.(*kerrors.StatusError).ErrStatus.Details; details != nil {
						fmt.Fprintf(errout, "Invalid %s %s\n", details.Kind, details.Name)
						for _, cause := range details.Causes {
							fmt.Fprintf(errout, "  %s: %s\n", cause.Field, cause.Message)
						}
						os.Exit(255)
					}
				}
				klog.Fatal(err)
			}
		},
	}

	flags := cmd.Flags()
	options.RecommendedOptions.AddFlags(flags)

	return cmd
}

// Validate validates the options for the HiveAPIServer command.
func (o *HiveAPIServer) Validate() error {
	return nil
}

// RunAPIServer takes the options, starts the API server and waits until stopCh is closed or initial listening fails.
func (o *HiveAPIServer) RunAPIServer(stopCh <-chan struct{}) error {
	return RunHiveAPIServer(o.RecommendedOptions, stopCh)
}
