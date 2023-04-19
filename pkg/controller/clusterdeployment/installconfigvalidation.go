package clusterdeployment

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	noAWSPlatformErr     = "install config did not contain an AWS platform"
	noGCPPlatformErr     = "install config did not contain a GCP platform"
	noAzurePlatformErr   = "install config did not contain an Azure platform"
	novSpherePlatformErr = "install config did not contain an vSphere platform"
	regionMismatchErr    = "install config region does not match cluster deployment region"

	missingvSphereCredentialsErr = "install config does not contain username/password for vSphere platform"
)

func ValidateInstallConfig(cd *hivev1.ClusterDeployment, installConfig []byte) error {

	ic := &installertypes.InstallConfig{}
	if err := yaml.Unmarshal(installConfig, &ic); err != nil {
		return errors.Wrap(err, "could not unmarshal InstallConfig")
	}

	switch platform := cd.Spec.Platform; {
	case platform.AWS != nil:
		if ic.Platform.AWS == nil {
			return errors.New(noAWSPlatformErr)
		}
		if ic.Platform.AWS.Region != cd.Spec.Platform.AWS.Region {
			return errors.New(regionMismatchErr)
		}
	case platform.GCP != nil:
		if ic.Platform.GCP == nil {
			return errors.New(noGCPPlatformErr)
		}
		if ic.Platform.GCP.Region != cd.Spec.Platform.GCP.Region {
			return errors.New(regionMismatchErr)
		}
	case platform.Azure != nil:
		if ic.Platform.Azure == nil {
			return errors.New(noAzurePlatformErr)
		}
		if ic.Platform.Azure.Region != cd.Spec.Platform.Azure.Region {
			return errors.New(regionMismatchErr)
		}
	case platform.VSphere != nil:
		if ic.Platform.VSphere == nil {
			return errors.New(novSpherePlatformErr)
		}
		if ic.Platform.VSphere.Username == "" || ic.Platform.VSphere.Password == "" {
			return errors.New(missingvSphereCredentialsErr)
		}
	}
	return nil
}
