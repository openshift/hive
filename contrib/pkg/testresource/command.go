package testresource

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/hive/pkg/resource"
)

// NewTestResourceCommand returns a command to test resource functions
func NewTestResourceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource SUB-COMMAND",
		Short: "Utility to test resource commands (apply/patch)",
		Long:  "Contains utility commands to test the resource patch and apply functions",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(newApplyCommand())
	cmd.AddCommand(newPatchCommand())

	return cmd
}

func mustRead(file string) []byte {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("erorr reading file %s: %v", file, err)
	}
	return content
}

func newApplyCommand() *cobra.Command {
	kubeconfigPath := ""
	cmd := &cobra.Command{
		Use:   "apply RESOURCEFILE",
		Short: "apply the given resource to the cluster",
		Long:  "Tests the Hive resource.Apply function",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Please specify a file with a resource to apply\n")
				cmd.Usage()
				return
			}
			if len(kubeconfigPath) == 0 {
				fmt.Printf("Please specify a Kubeconfig to use\n")
				cmd.Usage()
				return
			}
			content := mustRead(args[0])
			kubeconfig := mustRead(kubeconfigPath)
			helper := resource.NewHelper(kubeconfig, log.WithField("cmd", "apply"))
			info, err := helper.Info(content)
			if err != nil {
				fmt.Printf("Error obtaining info: %v\n", err)
				return
			}
			name := types.NamespacedName{Namespace: info.Namespace, Name: info.Name}
			fmt.Printf("The resource is %s (Kind: %s, APIVersion: %s)", name.String(), info.Kind, info.APIVersion)
			applyResult, err := helper.Apply(content)
			if err != nil {
				fmt.Printf("Error applying: %v\n", err)
				return
			}
			fmt.Printf("The resource was applied successfully: %s\n", applyResult)
		},
	}
	cmd.Flags().StringVarP(&kubeconfigPath, "kubeconfig", "k", os.Getenv("KUBECONFIG"), "Kubeconfig file to connect to target server")
	return cmd
}

func newPatchCommand() *cobra.Command {
	var (
		kubeconfigPath,
		patchTypeStr,
		namespace,
		name,
		kind,
		apiVersion string
		patchTypes = map[string]types.PatchType{"json": types.JSONPatchType, "merge": types.MergePatchType, "strategic": types.StrategicMergePatchType}
	)
	cmd := &cobra.Command{
		Use:   "patch PATCHFILE",
		Short: "apply the given patch to the cluster",
		Long:  "Tests the Hive resource.Patch function",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Please specify a file with a patch to apply\n")
				cmd.Usage()
				return
			}
			if len(kubeconfigPath) == 0 {
				fmt.Printf("Please specify a Kubeconfig to use\n")
				cmd.Usage()
				return
			}
			_, ok := patchTypes[patchTypeStr]
			if !ok {
				fmt.Printf("Invalid patch type %s\n", patchTypeStr)
				cmd.Usage()
				return
			}
			if len(name) == 0 {
				fmt.Printf("name is required\n")
				cmd.Usage()
				return
			}
			if len(kind) == 0 {
				fmt.Printf("kind is required\n")
				cmd.Usage()
				return
			}
			if len(apiVersion) == 0 {
				fmt.Printf("apiVersion is required\n")
				cmd.Usage()
				return
			}
			content := mustRead(args[0])
			kubeconfig := mustRead(kubeconfigPath)
			helper := resource.NewHelper(kubeconfig, log.WithField("cmd", "patch"))
			err := helper.Patch(types.NamespacedName{Name: name, Namespace: namespace}, kind, apiVersion, content, patchTypeStr)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("The patch was applied successfully.\n")
		},
	}
	cmd.Flags().StringVarP(&kubeconfigPath, "kubeconfig", "k", os.Getenv("KUBECONFIG"), "Kubeconfig file to connect to target server")
	cmd.Flags().StringVar(&patchTypeStr, "type", "strategic", "Type of patch to apply. Available types are: json, merge, strategic")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace of resource to patch")
	cmd.Flags().StringVar(&name, "name", "", "Name of the resource to patch")
	cmd.Flags().StringVar(&kind, "kind", "", "Kind of the resource to patch")
	cmd.Flags().StringVar(&apiVersion, "api-version", "", "API version of the resource to patch (ie. hive.openshift.io/v1alpha1)")
	return cmd
}
