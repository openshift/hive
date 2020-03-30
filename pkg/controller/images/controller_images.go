package images

import (
	"os"

	corev1 "k8s.io/api/core/v1"
)

const (
	// HiveImageEnvVar is the optional environment variable that overrides the image to use
	// for provisioning/deprovisioning. Typically this is the same image used by the hive
	// operator and is set as an EnvVar on the deployment.
	HiveImageEnvVar = "HIVE_IMAGE"

	// DefaultHiveImage is the image to use for hive when an image is not specified via the
	// environment variable
	DefaultHiveImage = "registry.svc.ci.openshift.org/openshift/hive-v4.0:hive"

	// HiveImagePullPolicyEnvVar is the optional environment variable that overrides
	// the pull policy to use for pulling the hive image. Typically this is the same
	// pull policy used by the the hive operator and is set as an EnvVar on the deployment.
	HiveImagePullPolicyEnvVar = "HIVE_IMAGE_PULL_POLICY"
)

// GetHiveImage returns the hive image to use in controllers. Either the one
// specified in the environment variable or the hardcoded default.
func GetHiveImage() string {
	return envVarOrDefault(HiveImageEnvVar, DefaultHiveImage)
}

// GetHiveImagePullPolicy returns the policy to use when pulling the hive image.
// Either the one specified in the environment variable or the hardcoded default.
func GetHiveImagePullPolicy() corev1.PullPolicy {
	return corev1.PullPolicy(envVarOrDefault(HiveImagePullPolicyEnvVar, string(corev1.PullAlways)))
}

func envVarOrDefault(envVar string, defaultValue string) string {
	value, ok := os.LookupEnv(envVar)
	if ok {
		return value
	}
	return defaultValue
}
