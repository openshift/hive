package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/constants"

	installertypes "github.com/openshift/installer/pkg/types"
	azureinstallertypes "github.com/openshift/installer/pkg/types/azure"
)

const (
	azureCredFile     = "osServicePrincipal.json"
	azureInstanceType = "Standard_D4s_v3"
)

var _ CloudBuilder = (*AzureCloudBuilder)(nil)

// AzureCloudBuilder encapsulates cluster artifact generation logic specific to Azure.
type AzureCloudBuilder struct {
	// ServicePrincipal is the bytes from a service principal file, typically ~/.azure/osServicePrincipal.json.
	ServicePrincipal []byte

	// BaseDomainResourceGroupName is the resource group where the base domain for this cluster is configured.
	BaseDomainResourceGroupName string

	// Region is the Azure region to which to install the cluster.
	Region string

	// CloudName is the name of the Azure cloud environment which will be used for the cluster.
	CloudName hivev1azure.CloudEnvironment

	// ResourceGroupName is the resource group where the cluster will be installed.
	ResourceGroupName string
}

func NewAzureCloudBuilderFromSecret(credsSecret *corev1.Secret) *AzureCloudBuilder {
	azureSP := credsSecret.Data[constants.AzureCredentialsName]
	return &AzureCloudBuilder{
		ServicePrincipal: azureSP,
	}
}

func (p *AzureCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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
			azureCredFile: p.ServicePrincipal,
		},
	}
}

func (p *AzureCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		Azure: &hivev1azure.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region:                      p.Region,
			BaseDomainResourceGroupName: p.BaseDomainResourceGroupName,
			CloudName:                   p.CloudName,
		},
	}
}

func (p *AzureCloudBuilder) AddMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Azure = &hivev1azure.MachinePool{
		InstanceType: azureInstanceType,
		OSDisk: hivev1azure.OSDisk{
			DiskSizeGB: 128,
		},
	}

}

func (p *AzureCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	// Inject platform details into InstallConfig:
	ic.Platform = installertypes.Platform{
		Azure: &azureinstallertypes.Platform{
			Region:                      p.Region,
			BaseDomainResourceGroupName: p.BaseDomainResourceGroupName,
			CloudName:                   azureinstallertypes.CloudEnvironment(p.CloudName),
			ResourceGroupName:           p.ResourceGroupName,
		},
	}

	// Used for both control plane and workers.
	mpp := &azureinstallertypes.MachinePool{}
	ic.ControlPlane.Platform.Azure = mpp
	ic.Compute[0].Platform.Azure = mpp
}

func (p *AzureCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-azure-creds", o.Name)
}

func (p *AzureCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{}
}
