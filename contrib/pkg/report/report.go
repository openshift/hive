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
	"github.com/spf13/cobra"
)

// NewClusterReportCommand creates a command that generates and outputs the cluster report.
func NewClusterReportCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Execute reports on the data in a Hive cluster.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewProvisioningReportCommand())
	cmd.AddCommand(NewDeprovisioningReportCommand())
	return cmd
}
