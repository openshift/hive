package utils

import (
	"context"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/installer/pkg/types/vsphere"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// ValidateCredentialsForClusterDeployment will attempt to verify that the platform/cloud credentials
// for the given ClusterDeployment are valid.
// Note: It simply checks that the username/password (or equivalent) can authenticate,
// not that the credentials have any specific permissions.
func ValidateCredentialsForClusterDeployment(kubeClient client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (bool, error) {
	secret := &corev1.Secret{}

	switch getClusterPlatform(cd) {
	case constants.PlatformVSphere:
		secretKey := types.NamespacedName{Name: cd.Spec.Platform.VSphere.CredentialsSecretRef.Name, Namespace: cd.Namespace}
		if err := kubeClient.Get(context.TODO(), secretKey, secret); err != nil {
			logger.WithError(err).Error("failed to read in ClusterDeployment's platform creds")
			return false, err
		}
		return validateVSphereCredentials(cd.Spec.Platform.VSphere.VCenter,
			string(secret.Data[constants.UsernameSecretKey]),
			string(secret.Data[constants.PasswordSecretKey]))
	default:
		// If we have no platform-specific credentials verification
		// assume the creds are valid.
		return true, nil

	}
}

func validateVSphereCredentials(vcenter, username, password string) (bool, error) {

	_, _, err := vsphere.CreateVSphereClients(context.TODO(), vcenter, username, password)
	return err == nil, nil
}

// getClusterPlatform returns the platform of a given ClusterDeployment
func getClusterPlatform(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AWS != nil:
		return constants.PlatformAWS
	case cd.Spec.Platform.Azure != nil:
		return constants.PlatformAzure
	case cd.Spec.Platform.GCP != nil:
		return constants.PlatformGCP
	case cd.Spec.Platform.OpenStack != nil:
		return constants.PlatformOpenStack
	case cd.Spec.Platform.VSphere != nil:
		return constants.PlatformVSphere
	case cd.Spec.Platform.BareMetal != nil:
		return constants.PlatformBaremetal
	}
	return constants.PlatformUnknown
}
