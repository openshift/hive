package clusterdeployment

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	noAWSPlatformErr   = "install config did not contain an AWS platform"
	noGCPPlatformErr   = "install config did not contain a GCP platform"
	noAzurePlatformErr = "install config did not contain an Azure platform"
	regionMismatchErr  = "install config region does not match cluster deployment region"
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
	}
	return nil
}
