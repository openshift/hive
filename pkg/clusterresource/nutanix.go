package clusterresource

import (
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/utils/nutanixutils"
	installertypes "github.com/openshift/installer/pkg/types"
	installernutanix "github.com/openshift/installer/pkg/types/nutanix"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ CloudBuilder = (*NutanixCloudBuilder)(nil)

type NutanixCloudBuilder struct {
	// PrismCentral is the endpoint (address and port) to connect to the Prism Central.
	// This serves as the default Prism-Central.
	PrismCentral installernutanix.PrismCentral `json:"prismCentral"`

	// FailureDomains configures failure domains for the Nutanix platform.
	// Required for using MachinePools
	FailureDomains []installernutanix.FailureDomain `json:"failureDomains"`

	// APIVIP is the virtual IP address for the api endpoint
	APIVIP string

	// IngressVIP is the virtual IP address for ingress
	IngressVIP string
}

func (p *NutanixCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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
		StringData: map[string]string{
			constants.UsernameSecretKey: p.PrismCentral.Username,
			constants.PasswordSecretKey: p.PrismCentral.Password,
		},
	}
}

func (p *NutanixCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{}
}

func (p *NutanixCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	failureDomains, _, _ := nutanixutils.ConvertInstallerFailureDomains(p.FailureDomains)

	return hivev1.Platform{
		Nutanix: &hivev1nutanix.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			PrismCentral: hivev1nutanix.PrismEndpoint{
				Address: p.PrismCentral.Endpoint.Address,
				Port:    p.PrismCentral.Endpoint.Port,
			},
			FailureDomains: failureDomains,
		},
	}
}

func (p *NutanixCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Nutanix = &hivev1nutanix.MachinePool{
		NumCPUs:           4,
		NumCoresPerSocket: 2,
		MemoryMiB:         16348,
		OSDisk: hivev1nutanix.OSDisk{
			DiskSizeGiB: volumeSize,
		},
		BootType: machinev1.NutanixLegacyBoot,
	}
}

func (p *NutanixCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	prismElements, subnetUUIDs := nutanixutils.ExtractInstallerResources(p.FailureDomains)

	ic.Platform = installertypes.Platform{
		Nutanix: &installernutanix.Platform{
			PrismCentral: installernutanix.PrismCentral{
				Endpoint: installernutanix.PrismEndpoint{
					Address: p.PrismCentral.Endpoint.Address,
					Port:    p.PrismCentral.Endpoint.Port,
				},
				Username: p.PrismCentral.Username,
				Password: p.PrismCentral.Password,
			},
			PrismElements:  prismElements,
			APIVIPs:        []string{p.APIVIP},
			IngressVIPs:    []string{p.IngressVIP},
			SubnetUUIDs:    subnetUUIDs,
			FailureDomains: p.FailureDomains,
		},
	}
}

func (p *NutanixCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-nutanix-creds", o.Name)
}
