package common

import (
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/hive"
	log "github.com/sirupsen/logrus"
	"os"
)

func GetHiveNamespaceOrDie() string {
	hiveNS := os.Getenv(constants.HiveNamespaceEnvVar)
	if len(hiveNS) == 0 {
		log.Fatalf("required environment variable is not defined: %s", constants.HiveNamespaceEnvVar)
	}
	return hiveNS
}

func GetHiveOperatorNamespaceOrDie() string {
	namespace := os.Getenv(hive.HiveOperatorNamespaceEnvVar)
	if len(namespace) == 0 {
		log.Fatalf("required environment variable is not defined: %s", hive.HiveOperatorNamespaceEnvVar)
	}
	return namespace
}
