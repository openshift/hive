package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1baremetal "github.com/openshift/hive/pkg/apis/hive/v1/baremetal"
)

var _ CloudBuilder = (*LocalCloudBuilder)(nil)

// LocalCloudBuilder encapsulates cluster artifact generation logic specific to a local cluster.
type LocalCloudBuilder struct {
}

func (p *LocalCloudBuilder) generateCredentialsSecret(o *Builder) *corev1.Secret {
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
			"local-kubeconfig": o.AdoptAdminKubeconfig,
		},
	}
}

func (p *LocalCloudBuilder) addClusterDeploymentPlatform(o *Builder, cd *hivev1.ClusterDeployment) {
	cd.Spec.Platform = hivev1.Platform{
		BareMetal: &hivev1baremetal.Platform{},
	}
}

func (p *LocalCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	// Nothing to do here
}

func (p *LocalCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	// Nothing to do here
}

func (p *LocalCloudBuilder) credsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-local-creds", o.Name)
}
