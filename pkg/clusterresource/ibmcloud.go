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

const (
	ibmInstanceType = "bx2-4x16"
)

var _ CloudBuilder = (*IBMCloudBuilder)(nil)

// IBMCloudBuilder encapsulates cluster artifact generation logic specific to IBM Cloud.
type IBMCloudBuilder struct {
	// AccountID is the IBM Cloud Account ID
	AccountID string `json:"accountID"`

	// CISInstanceCRN is the IBM Cloud Internet Services Instance CRN
	CISInstanceCRN string `json:"cisInstanceCRN"`

	// APIKey is the IBM Cloud api key.
	APIKey string

	// Region specifies the IBM Cloud region where the cluster will be
	// created.
	Region string `json:"region"`
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
			AccountID:      p.AccountID,
			CISInstanceCRN: p.CISInstanceCRN,
			Region:         p.Region,
		},
	}
}

func (p *IBMCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.IBMCloud = &hivev1ibmcloud.MachinePool{
		InstanceType: ibmInstanceType,
	}
}

func (p *IBMCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		IBMCloud: &installeribmcloud.Platform{
			Region: p.Region,
		},
	}

	// IBM Cloud only supports manual credentials mode. Manifests including required secrets
	// must be passed to hive via cd.spec.provisioning.manifestsConfigmapRef
	ic.CredentialsMode = installertypes.ManualCredentialsMode
}

func (p *IBMCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-ibm-creds", o.Name)
}
