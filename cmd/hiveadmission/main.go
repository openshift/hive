package main

import (
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivevalidatingwebhooks "github.com/openshift/hive/pkg/validating-webhooks/hive/v1"
	"github.com/openshift/hive/pkg/version"
)

func main() {
	log.Infof("Version: %s", version.String())
	log.Info("Starting CRD Validation Webhooks.")

	// TODO: figure out a way to combine logrus and klog logging levels. The team has decided that hardcoding this is ok for now.
	log.SetLevel(log.InfoLevel)

	decoder := createDecoder()

	admissionCmd.RunAdmissionServer(
		hivevalidatingwebhooks.NewDNSZoneValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewClusterDeploymentValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewClusterPoolValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewClusterImageSetValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewClusterProvisionValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewMachinePoolValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewSyncSetValidatingAdmissionHook(decoder),
		hivevalidatingwebhooks.NewSelectorSyncSetValidatingAdmissionHook(decoder),
	)
}

func createDecoder() *admission.Decoder {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	decoder, err := admission.NewDecoder(scheme)
	if err != nil {
		log.WithError(err).Fatal("could not create a decoder")
	}
	return decoder
}
