package main

import (
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	log "github.com/sirupsen/logrus"

	hivevalidatingwebhooks "github.com/openshift/hive/pkg/apis/hive/v1/validating-webhooks"
	"github.com/openshift/hive/pkg/certs"
)

func main() {
	log.Info("Starting CRD Validation Webhooks.")

	// TODO: figure out a way to combine logrus and klog logging levels. The team has decided that hardcoding this is ok for now.
	log.SetLevel(log.InfoLevel)

	// TODO: Would be better to get this from the --tls-cert-file argument. The parsing of the argument is hidden in
	// the internals of RunAdmissionServer.
	const certsDir = "/var/serving-cert"
	certs.TerminateOnCertChanges(certsDir)

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
