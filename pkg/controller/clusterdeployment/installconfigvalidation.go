package clusterdeployment

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	installertypes "github.com/openshift/installer/pkg/types"
	corev1 "k8s.io/api/core/v1"

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

// ValidateInstallConfig ensures that the "install-config.yaml" in the `installConfigSecret`
// unmarshals correctly as an InstallConfig; validates that its Platform is consistent with
// that of the `cd`; and checks for other required fields.
// The first return is the InstallConfig object itself. This is `nil` only if unmarshalling
// failed -- i.e. we still attempt to return a replete InstallConfig if post-unmarshalling
// validation checks fail.
// The second return indicates the nature of the validation failure. If `nil`, we consider
// the `installConfigSecret` "valid".`
func ValidateInstallConfig(cd *hivev1.ClusterDeployment, installConfigSecret *corev1.Secret) (*installertypes.InstallConfig, error) {

	ic := &installertypes.InstallConfig{}
	if err := yaml.Unmarshal(installConfigSecret.Data["install-config.yaml"], &ic); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal InstallConfig")
	}

	switch platform := cd.Spec.Platform; {
	case platform.AWS != nil:
		if ic.Platform.AWS == nil {
			return ic, errors.New(noAWSPlatformErr)
		}
		if ic.Platform.AWS.Region != cd.Spec.Platform.AWS.Region {
			return ic, errors.New(regionMismatchErr)
		}
	case platform.GCP != nil:
		if ic.Platform.GCP == nil {
			return ic, errors.New(noGCPPlatformErr)
		}
		if ic.Platform.GCP.Region != cd.Spec.Platform.GCP.Region {
			return ic, errors.New(regionMismatchErr)
		}
	case platform.Azure != nil:
		if ic.Platform.Azure == nil {
			return ic, errors.New(noAzurePlatformErr)
		}
		if ic.Platform.Azure.Region != cd.Spec.Platform.Azure.Region {
			return ic, errors.New(regionMismatchErr)
		}
	case platform.VSphere != nil:
		if ic.Platform.VSphere == nil {
			return ic, errors.New(novSpherePlatformErr)
		}
		if (ic.Platform.VSphere.DeprecatedUsername == "" || ic.Platform.VSphere.DeprecatedPassword == "") &&
			(len(ic.Platform.VSphere.VCenters) == 0 || ic.Platform.VSphere.VCenters[0].Username == "" ||
				ic.Platform.VSphere.VCenters[0].Password == "") {
			return ic, errors.New(missingvSphereCredentialsErr)
		}
	}
	return ic, nil
}
