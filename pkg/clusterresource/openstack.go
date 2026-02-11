package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installeropenstack "github.com/openshift/installer/pkg/types/openstack"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*OpenStackCloudBuilder)(nil)

// OpenStackCloudBuilder encapsulates cluster artifact generation logic specific to OpenStack.
type OpenStackCloudBuilder struct {
	// APIFloatingIP is the OpenStack Floating IP for the cluster to use for its API
	APIFloatingIP string

	// IngressFloatingIP is the OpenStack Floating IP for the cluster to use for its Ingress
	IngressFloatingIP string

	// Cloud is the named section from the clouds.yaml in the Secret containing the creds.
	Cloud string

	// CloudsYAMLContent is the data that will be used as the clouds.yaml file for
	// cluster provisioning.
	CloudsYAMLContent []byte

	// ExternalNetwork is the OpenStack network to install the cluster into.
	ExternalNetwork string

	// ComputeFlavor is the OpenStack flavor type to use for workers and to set
	// the default for other machine pools.
	ComputeFlavor string

	// MasterFlavor is the OpenStack flavor type to use for master instances.
	MasterFlavor string
}

func NewOpenStackCloudBuilderFromSecret(credsSecret *corev1.Secret) *OpenStackCloudBuilder {
	cloudsYamlContent := credsSecret.Data[constants.OpenStackCredentialsName]
	return &OpenStackCloudBuilder{
		CloudsYAMLContent: cloudsYamlContent,
	}
}

func (p *OpenStackCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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
			constants.OpenStackCredentialsName: p.CloudsYAMLContent,
		},
	}
}

func (p *OpenStackCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		OpenStack: &hivev1openstack.Platform{
			Cloud: p.Cloud,
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
		},
	}
}

func (p *OpenStackCloudBuilder) AddMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.OpenStack = &hivev1openstack.MachinePool{
		Flavor: p.ComputeFlavor,
	}
}

func (p *OpenStackCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		OpenStack: &installeropenstack.Platform{
			Cloud:                p.Cloud,
			ExternalNetwork:      p.ExternalNetwork,
			DeprecatedFlavorName: p.ComputeFlavor,
			APIFloatingIP:        p.APIFloatingIP,
			IngressFloatingIP:    p.IngressFloatingIP,
		},
	}

	ic.Compute[0].Platform.OpenStack = &installeropenstack.MachinePool{
		FlavorName: p.ComputeFlavor,
	}
	ic.ControlPlane.Platform.OpenStack = &installeropenstack.MachinePool{
		FlavorName: p.MasterFlavor,
	}
}

func (p *OpenStackCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-openstack-creds", o.Name)
}

func (p *OpenStackCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{}
}
