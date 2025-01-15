package clusterresource

import (
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installergcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	gcpInstanceType = "n1-standard-4"
)

var _ CloudBuilder = (*GCPCloudBuilder)(nil)

// GCPCloudBuilder encapsulates cluster artifact generation logic specific to GCP.
type GCPCloudBuilder struct {
	// ServicePrincipal is the bytes from a service account file, typically ~/.gcp/osServiceAccount.json.
	ServiceAccount []byte

	// ProjectID is the GCP project to use.
	ProjectID string

	// Region is the GCP region to which to install the cluster.
	Region string

	// PrivateServiceConnect is true if the cluster should use GCP Private Service Connect
	PrivateServiceConnect bool

	// DiscardLocalSsdOnHibernate describes the desired behavior of SSDs attached to certain
	// VM types when instances are shut down on hibernate. See the field of the same name in
	// the GCP Platform API.
	DiscardLocalSsdOnHibernate *bool
}

func NewGCPCloudBuilderFromSecret(credsSecret *corev1.Secret) (*GCPCloudBuilder, error) {
	gcpSA := credsSecret.Data[constants.GCPCredentialsName]
	projectID, err := gcpclient.ProjectID(gcpSA)
	if err != nil {
		return nil, errors.Wrap(err, "error loading GCP project ID from service account json")
	}
	return &GCPCloudBuilder{
		ServiceAccount: gcpSA,
		ProjectID:      projectID,
	}, nil
}

func (p *GCPCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.CredsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			constants.GCPCredentialsName: p.ServiceAccount,
		},
	}
}

func (p *GCPCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region: p.Region,
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				Enabled: p.PrivateServiceConnect,
			},
			// May be nil
			DiscardLocalSsdOnHibernate: p.DiscardLocalSsdOnHibernate,
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
			Region:    p.Region,
		},
	}

	// Used for both control plane and workers.
	mpp := &installergcp.MachinePool{
		InstanceType: gcpInstanceType,
	}
	ic.ControlPlane.Platform.GCP = mpp
	ic.Compute[0].Platform.GCP = mpp
}

func (p *GCPCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-gcp-creds", o.Name)
}

func (p *GCPCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{}
}
