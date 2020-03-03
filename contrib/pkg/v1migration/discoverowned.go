package v1migration

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// DiscoverOwnedOptions is the set of options for discovering resources with owner references to Hive resources.
type DiscoverOwnedOptions struct {
	listLimit int64
}

// NewDiscoverOwnedCommand creates a command that executes the migration utility to discover resources with owner
// references to Hive resources.
func NewDiscoverOwnedCommand() *cobra.Command {
	opt := &DiscoverOwnedOptions{}
	cmd := &cobra.Command{
		Use:   "discover-owned",
		Short: "discover the resource with owner references to Hive resources",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	flags := cmd.Flags()
	flags.Int64Var(&opt.listLimit, "list-limit", 500, "Number of resources to fetch at a time")
	return cmd
}

// Run executes the command
func (o *DiscoverOwnedOptions) Run() error {
	clientConfig, err := contributils.GetClientConfig()
	if err != nil {
		return errors.Wrap(err, "could not get the client config")
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create a discovery client")
	}
	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return errors.Wrap(err, "could not discover the server resources")
	}
	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create kube client")
	}
	var errs []error
	for _, resourceList := range resources {
		for _, r := range resourceList.APIResources {
			if !canList(r) {
				continue
			}
			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "could not parse group version %q\n", resourceList.GroupVersion))
				continue
			}
			gvr := gv.WithResource(r.Name)
			keepLooking := true
			continueValue := ""
			for keepLooking {
				objs, err := client.Resource(gvr).List(metav1.ListOptions{
					Limit:    o.listLimit,
					Continue: continueValue,
				})
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "could not list resources %q\n", resourceList.GroupVersion))
					continue
				}
				for _, obj := range objs.Items {
					if isOwnedByHiveResource(&obj) {
						fmt.Println(gvr)
						keepLooking = false
						break
					}
				}
				continueValue = objs.GetContinue()
				if continueValue == "" {
					keepLooking = false
				}
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

func isOwnedByHiveResource(obj *unstructured.Unstructured) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.APIVersion == hivev1alpha1.SchemeGroupVersion.String() {
			return true
		}
	}
	return false
}

func canList(r metav1.APIResource) bool {
	for _, v := range r.Verbs {
		if v == "list" {
			return true
		}
	}
	return false
}
