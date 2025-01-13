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
		key := types.NamespacedName{Name: cd.Spec.Platform.VSphere.CredentialsSecretRef.Name, Namespace: cd.Namespace}
		if err := kubeClient.Get(context.TODO(), key, secret); err != nil {
			logger.WithError(err).Error("failed to read in ClusterDeployment's platform creds")
			return false, err
		}

		// We need to install certs for our client to be able to talk to the vcenter.
		// First load up the standard trusted CA bundle.
		cabCM := &corev1.ConfigMap{}
		key = types.NamespacedName{Namespace: cd.Namespace, Name: constants.TrustedCAConfigMapName}
		if err := kubeClient.Get(context.TODO(), key, cabCM); err != nil {
			logger.WithError(err).Error("failed to load trusted CA bundle ConfigMap")
			return false, err
		}
		trustedCABundlePath, err := createCAFile(([]byte)(cabCM.Data[constants.TrustedCABundleFile]))
		if trustedCABundlePath != "" {
			defer os.Remove(trustedCABundlePath)
		}
		if err != nil {
			logger.WithError(err).Error("failed to create trusted CA bundle file from ConfigMap")
			return false, err
		}
		rootCAFiles := []string{trustedCABundlePath}

		// Now load up configured certs.
		if cd.Spec.Platform.VSphere.CertificatesSecretRef.Name != "" {
			certificatesSecret := &corev1.Secret{}
			key = types.NamespacedName{Name: cd.Spec.Platform.VSphere.CertificatesSecretRef.Name, Namespace: cd.Namespace}
			if err := kubeClient.Get(context.TODO(), key, certificatesSecret); err != nil {
				logger.WithError(err).Error("failed to read in vSphere certificates")
				return false, err
			}

			for k, pem := range certificatesSecret.Data {
				path, err := createCAFile(pem)
				if path != "" {
					defer os.Remove(path)
				}
				if err != nil {
					logger.WithError(err).WithField("fileKey", k).Error("failed to create CA file")
					return false, err
				}
				rootCAFiles = append(rootCAFiles, path)
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

// createCAFile writes pem to a temporary file and returns its path.
// Caller is responsible for cleaning up the file.
func createCAFile(pem []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "rootcacerts")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	if _, err := tmpFile.Write(pem); err != nil {
		return tmpFile.Name(), err
	}
	return tmpFile.Name(), nil
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
