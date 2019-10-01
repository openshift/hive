package createcluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1alpha1/azure"
)

const (
	azureCredFile = "osServicePrincipal.json"
)

var _ cloudProvider = (*azureCloudProvider)(nil)

type azureCloudProvider struct {
}

func (p *azureCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	credsFilePath := filepath.Join(os.Getenv("HOME"), ".azure", azureCredFile)
	if l := os.Getenv("AZURE_AUTH_LOCATION"); l != "" {
		credsFilePath = l
	}
	if o.CredsFile != "" {
		credsFilePath = o.CredsFile
	}
	log.Infof("Loading Azure service principal from: %s", credsFilePath)
	spFileContents, err := ioutil.ReadFile(credsFilePath)
	if err != nil {
		return nil, err
	}

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
			azureCredFile: spFileContents,
		},
	}, nil
}

func (p *azureCloudProvider) addPlatformDetails(o *Options, cd *hivev1.ClusterDeployment) error {
	cd.Spec.Platform = hivev1.Platform{
		Azure: &hivev1azure.Platform{
			Region:                      "centralus",
			BaseDomainResourceGroupName: o.AzureBaseDomainResourceGroupName,
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		Azure: &hivev1azure.PlatformSecrets{
			Credentials: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
		},
	}
	return nil
}

func (p *azureCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-azure-creds", o.Name)
}
