package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	installertypes "github.com/openshift/installer/pkg/types"
	installeralibabacloud "github.com/openshift/installer/pkg/types/alibabacloud"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1alibabacloud "github.com/openshift/hive/apis/hive/v1/alibabacloud"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*AlibabaCloudBuilder)(nil)

// AlibabaCloudBuilder encapsulates cluster artifact generation logic specific to Alibaba Cloud.
type AlibabaCloudBuilder struct {
	// AccessKeyID is the Alibaba Cloud access key ID.
	AccessKeyID string
	// AccessKeySecret is the Alibaba Cloud access key secret.
	AccessKeySecret string
	// Region specifies the Alibaba Cloud region where the cluster will be
	// created.
	Region string `json:"region"`
}

func (p *AlibabaCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
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
			constants.AlibabaCloudAccessKeyIDSecretKey:     p.AccessKeyID,
			constants.AlibabaCloudAccessKeySecretSecretKey: p.AccessKeySecret,
		},
	}
}

func (p *AlibabaCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return nil
}

func (p *AlibabaCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		AlibabaCloud: &hivev1alibabacloud.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region: p.Region,
		},
	}
}

func (p *AlibabaCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.AlibabaCloud = &hivev1alibabacloud.MachinePool{
		InstanceType: installeralibabacloud.DefaultWorkerInstanceType,
	}
}

func (p *AlibabaCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		AlibabaCloud: &installeralibabacloud.Platform{
			Region: p.Region,
		},
	}

	// Used for control plane
	mppMaster := &installeralibabacloud.MachinePool{
		InstanceType:       installeralibabacloud.DefaultMasterInstanceType,
		SystemDiskCategory: installeralibabacloud.DefaultDiskCategory,
		SystemDiskSize:     installeralibabacloud.DefaultSystemDiskSize,
	}
	// Used for workers
	mppWorker := &installeralibabacloud.MachinePool{
		InstanceType:       installeralibabacloud.DefaultWorkerInstanceType,
		SystemDiskCategory: installeralibabacloud.DefaultDiskCategory,
		SystemDiskSize:     installeralibabacloud.DefaultSystemDiskSize,
	}
	ic.ControlPlane.Platform.AlibabaCloud = mppMaster
	ic.Compute[0].Platform.AlibabaCloud = mppWorker
}

func (p *AlibabaCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-alibabacloud-creds", o.Name)
}
