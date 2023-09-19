package report

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	contributils "github.com/openshift/hive/contrib/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProvisioningReportOptions is the set of options for the desired report.
type ProvisioningReportOptions struct {
	// AgeLT is a duration to filter to cluster created less than this duration ago.
	AgeLT string
	// AgeGT is a duration to filter to cluster created more than this duration ago.
	AgeGT string
	// ClusterType filters the report to only clusters of the given type.
	ClusterType string
}

// NewProvisioningReportCommand creates a command that generates and outputs the cluster report.
func NewProvisioningReportCommand() *cobra.Command {

	opt := &ProvisioningReportOptions{}
	cmd := &cobra.Command{
		Use:   "provisioning",
		Short: "Prints a report on all clusters currently provisioning",
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
	flags.StringVarP(&opt.AgeLT, "age-lt", "", "", "Only include clusters created less than this duration ago. (i.e. 24h)")
	flags.StringVarP(&opt.AgeGT, "age-gt", "", "", "Only include clusters created more than this duration ago. (i.e. 24h)")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *ProvisioningReportOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *ProvisioningReportOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *ProvisioningReportOptions) Run(dynClient client.Client) error {
	var ageLT, ageGT *time.Duration
	var err error
	if o.AgeLT != "" {
		d, err := time.ParseDuration(o.AgeLT)
		if err != nil {
			return err
		}
		ageLT = &d
	}
	if o.AgeGT != "" {
		d, err := time.ParseDuration(o.AgeGT)
		if err != nil {
			return err
		}
		ageGT = &d
	}

	cdList := &hivev1.ClusterDeploymentList{}
	err = dynClient.List(context.Background(), cdList)
	if err != nil {
		log.WithError(err).Fatal("error listing cluster deployments")
	}
	fmt.Printf("Loaded %d total clusters\n", len(cdList.Items))

	var total, installed, provisioning int
	for _, cd := range cdList.Items {
		total++
		if cd.Spec.Installed {
			installed++
			continue
		}
		if cd.DeletionTimestamp != nil {
			continue
		}
		if o.ClusterType != "" {
			ct, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
			if !ok || ct != o.ClusterType {
				continue
			}
		}

		if ageLT != nil && time.Since(cd.CreationTimestamp.Time) > *ageLT {
			fmt.Printf("\n\nSkipping cluster due to LT filter: %s\n", cd.Name)
			continue
		}
		if ageGT != nil && time.Since(cd.CreationTimestamp.Time) < *ageGT {
			fmt.Printf("\n\nSkipping cluster due to GT filter: %s\n", cd.Name)
			continue
		}

		provisioningFor := time.Since(cd.CreationTimestamp.Time).Seconds() / 60 / 60

		provisioning++

		ct, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
		if !ok {
			ct = "unspecified"
		}

		fmt.Printf("\n\nCluster: %s\n", cd.Name)
		fmt.Printf("Namespace: %s\n", cd.Namespace)
		fmt.Printf("Cluster type: %s\n", ct)
		fmt.Printf("Created: %s\n", cd.CreationTimestamp.Time)
		fmt.Printf("Provisioning for: %.2f hours\n", provisioningFor)
		fmt.Printf("Install Retries: %d\n", cd.Status.InstallRestarts)
		if cd.Spec.Provisioning.ImageSetRef != nil {
			fmt.Printf("ImageSet: %s\n", cd.Spec.Provisioning.ImageSetRef.Name)
		}
		cfgMap := &corev1.ConfigMap{}
		cfgMapName := fmt.Sprintf("%s-install-log", cd.Name)
		err := dynClient.Get(context.Background(),
			types.NamespacedName{
				Namespace: cd.Namespace,
				Name:      cfgMapName,
			},
			cfgMap)
		if err != nil {
			if errors.IsNotFound(err) {
				fmt.Println("No install log configmap found.")
			} else {
				// Can happen due to missing perms:
				fmt.Printf("Error looking up install log configmap: %s\n", err)
				continue
			}
		} else {
			installLog, ok := cfgMap.Data["log"]
			if !ok {
				log.Fatal("configmap missing log key")
			} else {
				fmt.Printf("%s\n", string(installLog))
			}
		}
	}

	fmt.Printf("%d clusters currently provisioning\n", provisioning)

	return nil
}
