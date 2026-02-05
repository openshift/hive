package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
)

var _ CloudBuilder = (*VSphereCloudBuilder)(nil)

// VSphereCloudBuilder encapsulates cluster artifact generation logic specific to vSphere.
type VSphereCloudBuilder struct {
	// This map is (at the time of this writing) expected to contain a subset of the following keys:
	// Old format:
	// - "username": string username
	// - "password": string cleartext password
	// New format:
	// - "vCenters": JSON string representing a github.com/openshift/installer/pkg/types/vsphere/VCenters
	CredsSecretData map[string][]byte

	// CACert is the CA certificate(s) used to communicate with the vCenter.
	CACert []byte

	// Infrastructure is the full vSphere platform spec
	// WARNING: Assume this contains creds! Access via the Infrastructure() getter.
	infrastructure *installervsphere.Platform
}

// Returns the VSphere Platform (install-config style) of the builder. If `clean` is true,
// we will scrub the credentials out of it. Do this e.g. if injecting into a non-Secret CR,
// but not e.g. if producing the install-config.yaml Secret, which (currently) is expected
// to contain credentials.
func (b *VSphereCloudBuilder) Infrastructure(clean bool) *installervsphere.Platform {
	if !clean {
		return b.infrastructure
	}
	// Scrub credentials
	i := b.infrastructure.DeepCopy()
	i.DeprecatedPassword = ""
	for _, v := range i.VCenters {
		v.Password = ""
	}
	return i
}

func NewVSphereCloudBuilder(creds map[string][]byte, certs []byte, infra *installervsphere.Platform) *VSphereCloudBuilder {
	return &VSphereCloudBuilder{
		CredsSecretData: creds,
		CACert:          certs,
		infrastructure:  infra,
	}
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
		Data: p.CredsSecretData,
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
			// Scrub creds -- this one is going in the CD
			Infrastructure: p.Infrastructure(true),
		},
	}
}

func (p *VSphereCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.VSphere = &hivev1vsphere.MachinePool{
		MachinePool: installervsphere.MachinePool{
			NumCPUs:           2,
			NumCoresPerSocket: 1,
			MemoryMiB:         8192,
			OSDisk: installervsphere.OSDisk{
				DiskSizeGB: 120,
			},
		},
	}
}

func (p *VSphereCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		// Preserve creds -- this one is going in the install-config Secret.
		VSphere: p.Infrastructure(false),
	}
}

func (p *VSphereCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-creds", o.Name)
}

func (p *VSphereCloudBuilder) certificatesSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-certs", o.Name)
}
