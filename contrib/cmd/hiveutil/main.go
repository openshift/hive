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

package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/createcluster"
	"github.com/openshift/hive/contrib/pkg/installmanager"
	"github.com/openshift/hive/contrib/pkg/testresource"
	"github.com/openshift/hive/contrib/pkg/verification"
	"github.com/openshift/hive/pkg/imageset"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	cmd := newCOUtilityCommand()

	err := cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}

func newCOUtilityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hiveutil SUB-COMMAND",
		Short: "Utilities for hive",
		Long:  "Contains various utilities for running and testing hive",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewDeprovisionAWSWithTagsCommand())
	cmd.AddCommand(verification.NewVerifyImportsCommand())
	cmd.AddCommand(installmanager.NewInstallManagerCommand())
	cmd.AddCommand(imageset.NewUpdateInstallerImageCommand())
	cmd.AddCommand(testresource.NewTestResourceCommand())
	cmd.AddCommand(createcluster.NewCreateClusterCommand())

	return cmd
}
