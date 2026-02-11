package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installeribmcloud "github.com/openshift/installer/pkg/types/ibmcloud"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*IBMCloudBuilder)(nil)

// IBMCloudBuilder encapsulates cluster artifact generation logic specific to IBM Cloud.
type IBMCloudBuilder struct {
	// APIKey is the IBM Cloud api key.
	APIKey string

	// Region specifies the IBM Cloud region where the cluster will be
	// created.
	Region string `json:"region"`

	// InstanceType specifies the IBM Cloud instance type
	InstanceType string `json:"instanceType"`
}

func (p *IBMCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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
			// This API KEY will be passed to the installer as constants.IBMCloudAPIKeyEnvVar
			constants.IBMCloudAPIKeySecretKey: p.APIKey,
		},
	}
}

func (p *IBMCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return nil
}

func (p *IBMCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		IBMCloud: &hivev1ibmcloud.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region: p.Region,
		},
	}
}

func (p *IBMCloudBuilder) AddMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.IBMCloud = &hivev1ibmcloud.MachinePool{
		InstanceType: p.InstanceType,
	}
}

func (p *IBMCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		IBMCloud: &installeribmcloud.Platform{
			Region: p.Region,
		},
	}

	// Used for both control plane and workers.
	if p.InstanceType != "" {
		mpp := &installeribmcloud.MachinePool{
			InstanceType: p.InstanceType,
		}
		ic.ControlPlane.Platform.IBMCloud = mpp
		ic.Compute[0].Platform.IBMCloud = mpp
	}
}

func (p *IBMCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-ibm-creds", o.Name)
}
