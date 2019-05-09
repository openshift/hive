/*
Copyright 2018 The Kubernetes Authors.

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

package report

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const longDesc = `
OVERVIEW
The hiveutil report command scans all ClusterDeployments created within the 
specified duration, and outputs useful information on success rates, retries,
and the errors encountered.
`

// Options is the set of options for the desired report.
type Options struct {
	// RecentDurationStr is a string duration which filters the report to clusters created within
	// this timeframe.
	RecentDurationStr string
	// RecentDuration is created by parsing RecentDurationStr.
	RecentDuration time.Duration
}

// NewClusterReportCommand creates a command that generates and outputs the cluster report.
func NewClusterReportCommand() *cobra.Command {

	opt := &Options{}
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Gathers and prints information on recent clusters.",
		Long:  longDesc,
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
	flags.StringVarP(&opt.RecentDurationStr, "duration", "d", "24h", "Process clusters created in last duration.")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *Options) Validate(cmd *cobra.Command) error {
	var err error
	o.RecentDuration, err = time.ParseDuration(o.RecentDurationStr)
	if err != nil {
		return err
	}
	return nil
}

// Run executes the command
func (o *Options) Run(dynClient client.Client) error {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	cdList := &hivev1.ClusterDeploymentList{}
	err := dynClient.List(context.Background(), &client.ListOptions{}, cdList)
	if err != nil {
		log.WithError(err).Fatal("error listing cluster deployments")
	}
	fmt.Printf("Loaded %d total clusters\n", len(cdList.Items))
	fmt.Printf("Filtering to those within last: %s\n", o.RecentDurationStr)

	var total, installed int
	for _, cd := range cdList.Items {
		if time.Since(cd.CreationTimestamp.Time) > o.RecentDuration {
			continue
		}
		total++
		if cd.Status.Installed {
			installed++
			continue
		}
		fmt.Printf("\n\nProcessing cluster: %s/%s\n", cd.Namespace, cd.Name)
		fmt.Printf("Created: %s\n", cd.CreationTimestamp.Time)
		fmt.Printf("ImageSet: %s\n", cd.Spec.ImageSet.Name)
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

	fmt.Printf("\n\n%d of %d clusters installed successfully in last %s\n",
		installed, total, o.RecentDurationStr)

	return nil
}
