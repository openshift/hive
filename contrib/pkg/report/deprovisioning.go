package report

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	contributils "github.com/openshift/hive/contrib/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeprovisioningReportOptions is the set of options for the desired report.
type DeprovisioningReportOptions struct {
	// ClusterType filters the report to only clusters of the given type.
	ClusterType string
}

// NewDeprovisioningReportCommand creates a command that generates and outputs the cluster report.
func NewDeprovisioningReportCommand() *cobra.Command {

	opt := &DeprovisioningReportOptions{}
	cmd := &cobra.Command{
		Use:   "deprovisioning",
		Short: "Prints a report on all clusters currently deprovisioning",
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				return
			}

			if err := opt.Validate(cmd); err != nil {
				return
			}

			dynClient, err := contributils.GetClient()
			if err != nil {
				log.WithError(err).Fatal("error creating kube clients")
			}

			err = opt.Run(dynClient)
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.ClusterType, "cluster-type", "", "", "Only include clusters with the given hive.openshift.io/cluster-type label.")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *DeprovisioningReportOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *DeprovisioningReportOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *DeprovisioningReportOptions) Run(dynClient client.Client) error {
	cdList := &hivev1.ClusterDeploymentList{}
	err := dynClient.List(context.Background(), cdList)
	if err != nil {
		log.WithError(err).Fatal("error listing cluster deployments")
	}
	fmt.Printf("Loaded %d total clusters\n", len(cdList.Items))

	var deprovisioning int
	for _, cd := range cdList.Items {
		if cd.DeletionTimestamp == nil {
			continue
		}

		ct, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
		if !ok {
			ct = "unspecified"
		}

		if o.ClusterType != "" && ct != o.ClusterType {
			continue
		}

		deprovisioning++

		deprovisioningFor := time.Since(cd.DeletionTimestamp.Time).Seconds() / 60 / 60

		fmt.Printf("\n\nCluster: %s\n", cd.Name)
		fmt.Printf("Namespace: %s\n", cd.Namespace)
		fmt.Printf("Cluster type: %s\n", ct)
		fmt.Printf("Created: %s\n", cd.CreationTimestamp.Time)
		fmt.Printf("Deleted: %s\n", cd.DeletionTimestamp.Time)
		fmt.Printf("Deprovisioning for: %.2f hours\n", deprovisioningFor)
		fmt.Println("Finalizers:")
		for _, f := range cd.Finalizers {
			fmt.Printf("  - %s\n", f)
		}
	}

	fmt.Printf("%d clusters currently deprovisioning\n", deprovisioning)

	return nil
}
