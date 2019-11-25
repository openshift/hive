package createcluster

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
	"github.com/openshift/hive/pkg/constants"
)

const (
	defaultInstanceType = "n1-standard-4"
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

func (p *gcpCloudProvider) addPlatformDetails(o *Options, cd *hivev1.ClusterDeployment) error {
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			ProjectID: o.GCPProjectID,
			Region:    "us-east1",
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		GCP: &hivev1gcp.PlatformSecrets{
			Credentials: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
		},
	}

	// Set default instance type for both control plane and workers.
	mpp := &hivev1gcp.MachinePool{
		InstanceType: defaultInstanceType,
	}

	cd.Spec.ControlPlane.Platform.GCP = mpp

	for i := range cd.Spec.Compute {
		cd.Spec.Compute[i].Platform.GCP = mpp
	}

	return nil
}

func (p *gcpCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-gcp-creds", o.Name)
}
