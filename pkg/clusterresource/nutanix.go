package clusterresource

import (
	"fmt"
	v1 "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	"github.com/openshift/hive/pkg/constants"
	installertypes "github.com/openshift/installer/pkg/types"
	installernutanix "github.com/openshift/installer/pkg/types/nutanix"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ CloudBuilder = (*VSphereCloudBuilder)(nil)

type NutanixCloudBuilder struct {
	// Endpoint is the URL of the Nutanix Prism Central instance.
	Endpoint string `json:"endpoint"`

	// Port is the port of the Nutanix Prism Central instance.
	Port int32 `json:"port"`

	// Username is the name of the user to use to connect to the Nutanix Prism Central instance.
	Username string

	// Password is the password for the user to use to connect to the Nutanix Prism Central instance.
	Password string

	// Cluster is the name of the Nutanix cluster to use for provisioning volumes.
	Cluster string `json:"cluster,omitempty"`

	// Subnet is the name of the subnet to use for provisioning volumes.
	Subnet string `json:"subnet,omitempty"`

	// APIVIP is the virtual IP address for the api endpoint
	APIVIP string

	// IngressVIP is the virtual IP address for ingress
	IngressVIP string

	// CACert is the CA certificate(s) used to communicate with the Nutanix Prism Central.
	CACert []byte
}

func NewNutanixCloudBuilder(credsSecret, certsSecret *corev1.Secret) *NutanixCloudBuilder {
	username := credsSecret.Data[constants.UsernameSecretKey]
	password := credsSecret.Data[constants.PasswordSecretKey]
	cacert := certsSecret.Data[".cacert"]

	return &NutanixCloudBuilder{
		Username: string(username),
		Password: string(password),
		CACert:   cacert,
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
			constants.UsernameSecretKey: p.Username,
			constants.PasswordSecretKey: p.Password,
		},
	}
}

func (p *NutanixCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{
		&corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.certificatesSecretName(o),
				Namespace: o.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				".cacert": p.CACert,
			},
		},
	}
}

func (p *NutanixCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		Nutanix: &hivev1nutanix.Platform{
			Endpoint: p.Endpoint,
			Port:     p.Port,
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			CertificatesSecretRef: corev1.LocalObjectReference{
				Name: p.certificatesSecretName(o),
			},
			Cluster: p.Cluster,
			Subnet:  p.Subnet,
		},
	}
}

func (p *NutanixCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Nutanix = &hivev1nutanix.MachinePool{
		NumVcpusPerSocket: 1,
		NumSockets:        2,
		MemorySizeMiB:     8192,
		Disks: []hivev1nutanix.Disk{
			{
				DiskSizeBytes: 128849018880,
				DeviceType:    v1.NutanixDiskDeviceTypeDisk,
				AdapterType:   v1.NutanixDiskAdapterTypeSATA,
			},
		},
	}
}

func (p *NutanixCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		Nutanix: &installernutanix.Platform{
			PrismCentral: installernutanix.PrismCentral{
				Endpoint: installernutanix.PrismEndpoint{
					Address: p.Endpoint,
					Port:    p.Port,
				},
				Username: p.Username,
				Password: p.Password,
			},
			//PrismElements:          nil,
			APIVIPs:     []string{p.APIVIP},
			IngressVIPs: []string{p.IngressVIP},
			SubnetUUIDs: []string{p.Subnet},
		},
	}
}

func (p *NutanixCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-nutanix-creds", o.Name)
}

func (p *NutanixCloudBuilder) certificatesSecretName(o *Builder) string {
	return fmt.Sprintf("%s-nutanix-certs", o.Name)
}
