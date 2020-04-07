package clusterresource

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	installertypes "github.com/openshift/installer/pkg/types"
	installergcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/constants"
)

const (
	gcpRegion       = "us-east1"
	gcpInstanceType = "n1-standard-4"
)

var _ CloudBuilder = (*GCPCloudBuilder)(nil)

// GCPCloudBuilder encapsulates cluster artifact generation logic specific to GCP.
type GCPCloudBuilder struct {
	// ServicePrincipal is the bytes from a service account file, typically ~/.gcp/osServiceAccount.json.
	ServiceAccount []byte

	// ProjectID is the GCP project to use.
	ProjectID string
}

func (p *GCPCloudBuilder) generateCredentialsSecret(o *Builder) *corev1.Secret {
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
			constants.GCPCredentialsName: p.ServiceAccount,
		},
	}
}

func (p *GCPCloudBuilder) addClusterDeploymentPlatform(o *Builder, cd *hivev1.ClusterDeployment) {
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
			Region: gcpRegion,
		},
	}
}

func (p *GCPCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.GCP = &hivev1gcp.MachinePool{
		InstanceType: gcpInstanceType,
	}

}

func (p *GCPCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		GCP: &installergcp.Platform{
			ProjectID: p.ProjectID,
			Region:    gcpRegion,
		},
	}

	// Used for both control plane and workers.
	mpp := &installergcp.MachinePool{
		InstanceType: gcpInstanceType,
	}
	ic.ControlPlane.Platform.GCP = mpp
	ic.Compute[0].Platform.GCP = mpp
}

func (p *GCPCloudBuilder) credsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-gcp-creds", o.Name)
}
