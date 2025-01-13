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

	// Infrastructure is the full vSphere platform spec
	Infrastructure *installervsphere.Platform
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
			Infrastructure: p.Infrastructure,
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
		VSphere: p.Infrastructure,
	}
}

func (p *VSphereCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-creds", o.Name)
}

func (p *VSphereCloudBuilder) certificatesSecretName(o *Builder) string {
	return fmt.Sprintf("%s-vsphere-certs", o.Name)
}
