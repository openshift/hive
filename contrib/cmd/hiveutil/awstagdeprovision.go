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
	"strings"

	"github.com/openshift/hive/contrib/pkg/awstagdeprovision"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &awstagdeprovision.ClusterUninstaller{}
	opt.Filters = awstagdeprovision.AWSFilter{}
	cmd := &cobra.Command{
		Use:   "aws-tag-deprovision KEY=VALUE ...",
		Short: "Deprovision AWS assets (as created by openshift-installer) with the given tag(s)",
		Long:  "Deprovision AWS assets (as created by openshift-installer) with the given tag(s).  A resource matches the filter if all of the key/value pairs are in its tags.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := completeAWSUninstaller(opt, args); err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Error("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LogLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region to use")
	flags.StringVar(&opt.ClusterName, "cluster-name", "", "Name that cluster was installed with")
	return cmd
}

func completeAWSUninstaller(o *awstagdeprovision.ClusterUninstaller, args []string) error {
	for _, arg := range args {
		err := parseFilter(o.Filters, arg)
		if err != nil {
			return fmt.Errorf("cannot parse filter %s: %v", arg, err)
		}
	}

	// Set log level
	level, err := log.ParseLevel(o.LogLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	o.Logger = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	return nil
}

func parseFilter(filterMap awstagdeprovision.AWSFilter, str string) error {
	parts := strings.SplitN(str, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("incorrectly formatted filter")
	}

	filterMap[parts[0]] = parts[1]

	return nil
}
