package utils

import (
	"context"
	"net/url"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// ValidateCredentialsForClusterDeployment will attempt to verify that the platform/cloud credentials
// for the given ClusterDeployment are valid.
// Note: It simply checks that the username/password (or equivalent) can authenticate,
// not that the credentials have any specific permissions.
func ValidateCredentialsForClusterDeployment(kubeClient client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (bool, error) {
	secret := &corev1.Secret{}

	switch GetClusterPlatform(cd) {
	case constants.PlatformVSphere:
		secretKey := types.NamespacedName{Name: cd.Spec.Platform.VSphere.CredentialsSecretRef.Name, Namespace: cd.Namespace}
		if err := kubeClient.Get(context.TODO(), secretKey, secret); err != nil {
			logger.WithError(err).Error("failed to read in ClusterDeployment's platform creds")
			return false, err
		}

		var rootCAFiles []string
		if cd.Spec.Platform.VSphere.CertificatesSecretRef.Name != "" {
			certificatesSecret := &corev1.Secret{}
			certificatesKey := types.NamespacedName{Name: cd.Spec.Platform.VSphere.CertificatesSecretRef.Name, Namespace: cd.Namespace}
			err := kubeClient.Get(context.TODO(), certificatesKey, certificatesSecret)
			if err != nil {
				logger.WithError(err).Error("failed to read in vSphere certificates")
				return false, err
			}

			rootCAFiles, err = createRootCAFiles(certificatesSecret)
			defer func() {
				for _, filename := range rootCAFiles {
					os.Remove(filename)
				}
			}()
			if err != nil {
				logger.WithError(err).Error("failed to create root CA files")
				return false, err
			}

		}

		for _, vcenter := range cd.Spec.Platform.VSphere.Infrastructure.VCenters {
			valid, err := validateVSphereCredentials(vcenter.Server,
				string(secret.Data[constants.UsernameSecretKey]),
				string(secret.Data[constants.PasswordSecretKey]),
				rootCAFiles,
				logger)
			if err != nil || valid == false {
				return false, err
			}
		}
		return true, nil
	default:
		// If we have no platform-specific credentials verification
		// assume the creds are valid.
		return true, nil

	}
}

// createRootCAFiles creates a temporary file for each key/value pair in the Secret's Data.
// Caller is responsible for cleaning up the created files.
func createRootCAFiles(certificateSecret *corev1.Secret) ([]string, error) {
	fileList := []string{}
	for _, fileContent := range certificateSecret.Data {
		tmpFile, err := os.CreateTemp("", "rootcacerts")
		if err != nil {
			return fileList, err
		}
		defer tmpFile.Close()

		fileList = append(fileList, tmpFile.Name())

		if _, err := tmpFile.Write(fileContent); err != nil {
			return fileList, err
		}
	}

	return fileList, nil
}

func validateVSphereCredentials(vcenter, username, password string, rootCAFiles []string, logger log.FieldLogger) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The high-level govmomi client doesn't allow us to set custom CAs early enough
	// so we need to reproduce a lot of the logic to allow setting things up properly
	// for the cases where a custom CA is needed.
	u, err := soap.ParseURL(vcenter)
	if err != nil {
		return false, err
	}

	u.User = url.UserPassword(username, password)

	soapClient := soap.NewClient(u, false)

	if len(rootCAFiles) > 0 {
		fileList := strings.Join(rootCAFiles, string(os.PathListSeparator))
		if err := soapClient.SetRootCAs(fileList); err != nil {
			logger.WithError(err).Error("failed to set vSphere root CAs")
			return false, err
		}
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		logger.WithError(err).Error("failed to create vSphere client")
		return false, err
	}

	restClient := rest.NewClient(vimClient)
	err = restClient.Login(ctx, u.User)
	if err != nil {
		logger.WithError(err).Warn("failed to authenticate into vSphere")
		return false, err
	}

	return true, nil
}

// GetClusterPlatform returns the platform of a given ClusterDeployment
func GetClusterPlatform(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AWS != nil:
		return constants.PlatformAWS
	case cd.Spec.Platform.Azure != nil:
		return constants.PlatformAzure
	case cd.Spec.Platform.BareMetal != nil:
		return constants.PlatformBaremetal
	case cd.Spec.Platform.GCP != nil:
		return constants.PlatformGCP
	case cd.Spec.Platform.IBMCloud != nil:
		return constants.PlatformIBMCloud
	case cd.Spec.Platform.Nutanix != nil:
		return constants.PlatformNutanix
	case cd.Spec.Platform.OpenStack != nil:
		return constants.PlatformOpenStack
	case cd.Spec.Platform.VSphere != nil:
		return constants.PlatformVSphere
	}
	return constants.PlatformUnknown
}
