package main

import (
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	log "github.com/sirupsen/logrus"

	hivevalidatingwebhooks "github.com/openshift/hive/pkg/apis/hive/v1/validating-webhooks"
)

func main() {
	log.Info("Starting CRD Validation Webhooks.")

	// TODO: figure out a way to combine logrus and klog logging levels. The team has decided that hardcoding this is ok for now.
	log.SetLevel(log.InfoLevel)

	admissionCmd.RunAdmissionServer(
		&hivevalidatingwebhooks.DNSZoneValidatingAdmissionHook{},
		hivevalidatingwebhooks.NewClusterDeploymentValidatingAdmissionHook(),
		&hivevalidatingwebhooks.ClusterImageSetValidatingAdmissionHook{},
		&hivevalidatingwebhooks.ClusterProvisionValidatingAdmissionHook{},
		&hivevalidatingwebhooks.MachinePoolValidatingAdmissionHook{},
		&hivevalidatingwebhooks.SyncSetValidatingAdmissionHook{},
		&hivevalidatingwebhooks.SelectorSyncSetValidatingAdmissionHook{},
	)
}
