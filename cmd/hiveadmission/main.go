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
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	log "github.com/sirupsen/logrus"

	hivevalidatingwebhooks "github.com/openshift/hive/pkg/apis/hive/v1alpha1/validating-webhooks"
)

func main() {
	log.Info("Starting CRD Validation Webhooks.")

	// TODO: figure out a way to combine logrus and glog logging levels. The team has decided that hardcoding this is ok for now.
	log.SetLevel(log.InfoLevel)

	admissionCmd.RunAdmissionServer(
		&hivevalidatingwebhooks.DNSZoneValidatingAdmissionHook{},
		hivevalidatingwebhooks.NewClusterDeploymentValidatingAdmissionHook(),
		&hivevalidatingwebhooks.ClusterImageSetValidatingAdmissionHook{},
	)
}
