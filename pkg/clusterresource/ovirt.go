package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	installertypes "github.com/openshift/installer/pkg/types"
	installerovirt "github.com/openshift/installer/pkg/types/ovirt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1ovirt "github.com/openshift/hive/pkg/apis/hive/v1/ovirt"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*OpenStackCloudBuilder)(nil)

// OvirtCloudBuilder encapsulates cluster artifact generation logic specific to oVirt.
type OvirtCloudBuilder struct {
	// OVirtConfigYAMLContent is the data that will be used as the ovirt-config.yaml file for
	// cluster provisioning.
	OVirtConfigYAMLContent []byte
	// The target cluster under which all VMs will run
	ClusterID string `json:"ovirt_cluster_id"`
	// The target storage domain under which all VM disk would be created.
	StorageDomainID string `json:"ovirt_storage_domain_id"`
	// The target network of all the network interfaces of the nodes. Omitting defaults to ovirtmgmt
	// network which is a default network for evert ovirt cluster.
	NetworkName string `json:"ovirt_network_name,omitempty"`
	// APIVIP is an IP which will be served by bootstrap and then pivoted masters, using keepalived
	APIVIP string `json:"api_vip"`
	// DNSVIP is the IP of the internal DNS which will be operated by the cluster
	DNSVIP string `json:"dns_vip"`
	// IngressIP is an external IP which routes to the default ingress controller.
	// The IP is a suitable target of a wildcard DNS record used to resolve default route host names.
	IngressVIP string `json:"ingress_vip"`
	// CACert is the CA certificate(s) used to communicate with oVirt.
	CACert []byte
}

func (p *OvirtCloudBuilder) generateCredentialsSecret(o *Builder) *corev1.Secret {
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
			constants.OvirtCredentialsName: p.OVirtConfigYAMLContent,
		},
	}
}

func (p *OvirtCloudBuilder) generateCloudCertificatesSecret(o *Builder) *corev1.Secret {
	return &corev1.Secret{
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
	}
}

func (p *OvirtCloudBuilder) addClusterDeploymentPlatform(o *Builder, cd *hivev1.ClusterDeployment) {
	cd.Spec.Platform = hivev1.Platform{
		Ovirt: &hivev1ovirt.Platform{
			ClusterID: p.ClusterID,
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
			CertificatesSecretRef: corev1.LocalObjectReference{
				Name: p.certificatesSecretName(o),
			},
		},
	}
}

func (p *OvirtCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Ovirt = &hivev1ovirt.MachinePool{}
}

func (p *OvirtCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		Ovirt: &installerovirt.Platform{
			ClusterID:       p.ClusterID,
			StorageDomainID: p.StorageDomainID,
			NetworkName:     p.NetworkName,
			APIVIP:          p.APIVIP,
			DNSVIP:          p.DNSVIP,
			IngressVIP:      p.IngressVIP,
		},
	}
}

func (p *OvirtCloudBuilder) credsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-ovirt-creds", o.Name)
}

func (p *OvirtCloudBuilder) certificatesSecretName(o *Builder) string {
	return fmt.Sprintf("%s-ovirt-certs", o.Name)
}
