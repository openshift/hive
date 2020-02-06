package createcluster

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	installertypes "github.com/openshift/installer/pkg/types"
	installergcp "github.com/openshift/installer/pkg/types/gcp"

	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	gcpRegion       = "us-east1"
	gcpInstanceType = "n1-standard-4"
)

var _ cloudProvider = (*gcpCloudProvider)(nil)

type gcpCloudProvider struct {
}

func (p *gcpCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	saFileContents, err := gcputils.GetCreds(o.CredsFile)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.credsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			constants.GCPCredentialsName: saFileContents,
		},
	}, nil
}

func (p *gcpCloudProvider) addPlatformDetails(
	o *Options,
	cd *hivev1.ClusterDeployment,
	machinePool *hivev1.MachinePool,
	installConfig *installertypes.InstallConfig,
) error {
	creds, err := gcputils.GetCreds(o.CredsFile)
	if err != nil {
		return err
	}
	projectID, err := gcpclient.ProjectID(creds)
	if err != nil {
		return err
	}

	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
			Region: gcpRegion,
		},
	}

	machinePool.Spec.Platform.GCP = &hivev1gcp.MachinePool{
		InstanceType: gcpInstanceType,
	}

	installConfig.Platform = installertypes.Platform{
		GCP: &installergcp.Platform{
			ProjectID: projectID,
			Region:    gcpRegion,
		},
	}

	// Used for both control plane and workers.
	mpp := &installergcp.MachinePool{
		InstanceType: gcpInstanceType,
	}
	installConfig.ControlPlane.Platform.GCP = mpp
	installConfig.Compute[0].Platform.GCP = mpp

	return nil
}

func (p *gcpCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-gcp-creds", o.Name)
}
