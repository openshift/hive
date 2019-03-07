package images

import (
	"os"

	log "github.com/sirupsen/logrus"
)

const (
	// HiveImageEnvVar is the optional environment variable that overrides the image to use
	// for provisioning/deprovisioning. Typically this originates from the HiveConfig and is
	// set as an EnvVar on the deployment.
	HiveImageEnvVar = "HIVE_IMAGE"

	// DefaultHiveImage is the image to use for hive when an image is not specified via the
	// environment variable
	DefaultHiveImage = "registry.svc.ci.openshift.org/openshift/hive-v4.0:hive"

	// CLIImageEnvVar is the optional environment variable that overrides the openshift CLI
	// image to use for extracting release information.
	CLIImageEnvVar = "CLI_IMAGE"

	// DefaultCLIImage is the image to use for the openshift CLI when an image is not specified
	// via the environment variable
	DefaultCLIImage = "registry.svc.ci.openshift.org/openshift/origin-v4.0:cli"
)

// GetHiveImage returns the hive image to use in controllers. Either the one
// specified in the environment variable or the hardcoded default.
func GetHiveImage(logger log.FieldLogger) string {
	hiveImage, ok := os.LookupEnv(HiveImageEnvVar)
	if !ok {
		logger.Debugf("using default hive image: %s", DefaultHiveImage)
		return DefaultHiveImage
	}
	logger.Debugf("using hive image from %s env var: %s", HiveImageEnvVar, hiveImage)
	return hiveImage
}

// GetCLIImage returns the CLI image to use in controllers. Either the one
// specified in the environment variable or the hardcoded default.
func GetCLIImage(logger log.FieldLogger) string {
	cliImage, ok := os.LookupEnv(CLIImageEnvVar)
	if !ok {
		logger.Debugf("using default cli image: %s", DefaultCLIImage)
		return DefaultCLIImage
	}
	logger.Debugf("using cli image from %s env var: %s", CLIImageEnvVar, cliImage)
	return cliImage
}
