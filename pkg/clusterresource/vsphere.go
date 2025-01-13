package clusterresource

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*VSphereCloudBuilder)(nil)

// VSphereCloudBuilder encapsulates cluster artifact generation logic specific to vSphere.
type VSphereCloudBuilder struct {
	// Username is the name of the user to use to connect to the vCenter.
	Username string

	// Password is the password for the user to use to connect to the vCenter.
	Password string

	// CACert is the CA certificate(s) used to communicate with the vCenter.
	CACert []byte

	// VSphere is the full vSphere platform spec
	VSphere *configv1.VSpherePlatformSpec
}

func NewVSphereCloudBuilderFromSecret(credsSecret, certsSecret *corev1.Secret) *VSphereCloudBuilder {
	username := credsSecret.Data[constants.UsernameSecretKey]
	password := credsSecret.Data[constants.PasswordSecretKey]
	cacert := certsSecret.Data[".cacert"]
	return &VSphereCloudBuilder{
		Username: string(username),
		Password: string(password),
		CACert:   cacert,
	}
}

func NewDummyVSphereCloudBuilder() *VSphereCloudBuilder {
	return &VSphereCloudBuilder{}
}

func (p *VSphereCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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

func (p *VSphereCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
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

func (p *VSphereCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		VSphere: &hivev1vsphere.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			CertificatesSecretRef: corev1.LocalObjectReference{
				Name: p.certificatesSecretName(o),
			},
			VSphere: p.VSphere,
		},
	}
}

func (p *VSphereCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.VSphere = &hivev1vsphere.MachinePool{
		NumCPUs:           2,
		NumCoresPerSocket: 1,
		MemoryMiB:         8192,
		OSDisk: hivev1vsphere.OSDisk{
			DiskSizeGB: 120,
		},
	}
}

func (p *VSphereCloudBuilder) AttachToInstallConfig(ic *installertypes.InstallConfig) {
	var vCenters []installervsphere.VCenter
	var failureDomains []installervsphere.FailureDomain
	var apiVIPs []string
	var ingressVIPs []string

	for _, vCenter := range p.VSphere.VCenters {
		vCenters = append(vCenters, installervsphere.VCenter{
			Server:      vCenter.Server,
			Username:    p.Username,
			Password:    p.Password,
			Datacenters: vCenter.Datacenters,
		})
	}

	for _, failureDomain := range p.VSphere.FailureDomains {
		failureDomains = append(failureDomains, installervsphere.FailureDomain{
			Server: failureDomain.Server,
			Name:   failureDomain.Name,
			Zone:   failureDomain.Zone,
			Region: failureDomain.Region,
			Topology: installervsphere.Topology{
				Datacenter:     failureDomain.Topology.Datacenter,
				ComputeCluster: failureDomain.Topology.ComputeCluster,
				Networks:       failureDomain.Topology.Networks,
				Datastore:      failureDomain.Topology.Datastore,
				ResourcePool:   failureDomain.Topology.ResourcePool,
				Folder:         failureDomain.Topology.Folder,
				Template:       failureDomain.Topology.Template,
			},
		})
	}

	for _, apiVIP := range p.VSphere.APIServerInternalIPs {
		apiVIPs = append(apiVIPs, string(apiVIP))
	}

	for _, ingressVIP := range p.VSphere.IngressIPs {
		ingressVIPs = append(ingressVIPs, string(ingressVIP))
	}

	ic.Platform = installertypes.Platform{
		VSphere: &installervsphere.Platform{
			VCenters:       vCenters,
			FailureDomains: failureDomains,
			APIVIPs:        apiVIPs,
			IngressVIPs:    ingressVIPs,
		},
	}
}

func (p *VSphereCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	p.AttachToInstallConfig(ic)
}

func (p *VSphereCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-creds", o.Name)
}

func (p *VSphereCloudBuilder) certificatesSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-certs", o.Name)
}

// APIPlatformSpecFromInstallerPlatformSpecAndIPs builds an openshift api VSpherePlatformSpec from the given installer platform spec
// and the two IP addresses (API and ingress).
func APIPlatformSpecFromInstallerPlatformSpecAndIPs(platform installervsphere.Platform, apisVIP, ingressVIP string) *configv1.VSpherePlatformSpec {
	vcenters := make([]configv1.VSpherePlatformVCenterSpec, 0, len(platform.VCenters))
	for _, vcenter := range platform.VCenters {
		vcenters = append(vcenters, configv1.VSpherePlatformVCenterSpec{
			Server:      vcenter.Server,
			Port:        vcenter.Port,
			Datacenters: vcenter.Datacenters,
		})
	}

	failureDomains := make([]configv1.VSpherePlatformFailureDomainSpec, 0, len(platform.FailureDomains))
	for _, failureDomain := range platform.FailureDomains {
		failureDomains = append(failureDomains, configv1.VSpherePlatformFailureDomainSpec{
			Server: failureDomain.Server,
			Name:   failureDomain.Name,
			Zone:   failureDomain.Zone,
			Region: failureDomain.Region,
			Topology: configv1.VSpherePlatformTopology{
				ResourcePool:   failureDomain.Topology.ResourcePool,
				ComputeCluster: failureDomain.Topology.ComputeCluster,
				Datacenter:     failureDomain.Topology.Datacenter,
				Datastore:      failureDomain.Topology.Datastore,
				Networks:       failureDomain.Topology.Networks,
			},
		})
	}

	return &configv1.VSpherePlatformSpec{
		VCenters:             vcenters,
		FailureDomains:       failureDomains,
		APIServerInternalIPs: []configv1.IP{configv1.IP(apisVIP)},
		IngressIPs:           []configv1.IP{configv1.IP(ingressVIP)},
	}
}
