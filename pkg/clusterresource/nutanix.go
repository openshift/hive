package clusterresource

import (
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	"github.com/openshift/hive/pkg/constants"
	installertypes "github.com/openshift/installer/pkg/types"
	installernutanix "github.com/openshift/installer/pkg/types/nutanix"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ CloudBuilder = (*NutanixCloudBuilder)(nil)

type NutanixCloudBuilder struct {
	// PrismCentral is the endpoint (address and port) and credentials to
	// connect to the Prism Central.
	// This serves as the default Prism-Central.
	PrismCentral installernutanix.PrismCentral `json:"prismCentral"`

	// PrismElements holds a list of Prism Elements (clusters). A Prism Element encompasses all Nutanix resources (VMs, subnets, etc.)
	// used to host the OpenShift cluster. Currently only a single Prism Element may be defined.
	// This serves as the default Prism-Element.
	PrismElements []installernutanix.PrismElement `json:"prismElements"`

	// SubnetUUIDs identifies the network subnets to be used by the cluster.
	// Currently, we only support one subnet for an OpenShift cluster.
	SubnetUUIDs []string `json:"subnetUUIDs"`

	// APIVIP is the virtual IP address for the api endpoint
	APIVIP string

	// IngressVIP is the virtual IP address for ingress
	IngressVIP string

	//// CACert is the CA certificate(s) used to communicate with the Nutanix Prism Central.
	//CACert []byte
}

func NewNutanixCloudBuilder(credsSecret *corev1.Secret) *NutanixCloudBuilder {
	username := credsSecret.Data[constants.UsernameSecretKey]
	password := credsSecret.Data[constants.PasswordSecretKey]

	return &NutanixCloudBuilder{
		PrismCentral: installernutanix.PrismCentral{
			Endpoint: installernutanix.PrismEndpoint{},
			Username: string(username),
			Password: string(password),
		},
	}
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
	var elements []hivev1nutanix.PrismElement
	for _, pe := range p.PrismElements {
		e := hivev1nutanix.PrismElement{
			UUID: pe.UUID,
			Endpoint: hivev1nutanix.PrismEndpoint{
				Address: pe.Endpoint.Address,
				Port:    pe.Endpoint.Port,
			},
			Name: pe.Name,
		}
		elements = append(elements, e)

	}

	return hivev1.Platform{
		Nutanix: &hivev1nutanix.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			PrismCentral: hivev1nutanix.PrismCentral{
				Endpoint: hivev1nutanix.PrismEndpoint{
					Address: p.PrismCentral.Endpoint.Address,
					Port:    p.PrismCentral.Endpoint.Port,
				},
			},
			PrismElements:          elements,
			ClusterOSImage:         "",  // TODO
			DefaultMachinePlatform: nil, // TODO
			SubnetUUIDs:            p.SubnetUUIDs,
			//FailureDomains:         nil, // TODO
		},
	}
}

func (p *NutanixCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Nutanix = &hivev1nutanix.MachinePool{
		NumCPUs:           2,
		NumCoresPerSocket: 2,
		MemoryMiB:         8192,
		OSDisk: hivev1nutanix.OSDisk{
			DiskSizeGiB: volumeSize,
		},
		BootType:       machinev1.NutanixUEFIBoot,
		Project:        nil, // TODO
		Categories:     nil, // TODO
		GPUs:           nil, // TODO
		DataDisks:      nil, // TODO
		FailureDomains: nil, // TODO
	}

}

func (p *NutanixCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
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
			//PrismElements:          nil,
			APIVIPs:     []string{p.APIVIP},
			IngressVIPs: []string{p.IngressVIP},
			SubnetUUIDs: p.SubnetUUIDs,
		},
	}
}

func (p *NutanixCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-nutanix-creds", o.Name)
}
