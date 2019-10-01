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
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
)

const (
	gcpCredFile = "osServiceAccount.json"
)

var _ cloudProvider = (*gcpCloudProvider)(nil)

type gcpCloudProvider struct {
}

func (p *gcpCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	credsFilePath := filepath.Join(os.Getenv("HOME"), ".gcp", gcpCredFile)
	if l := os.Getenv("GCP_SHARED_CREDENTIALS_FILE"); l != "" {
		credsFilePath = l
	}
	if o.CredsFile != "" {
		credsFilePath = o.CredsFile
	}
	log.Infof("Loading gcp service account from: %s", credsFilePath)
	saFileContents, err := ioutil.ReadFile(credsFilePath)
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
			gcpCredFile: saFileContents,
		},
	}, nil
}

func (p *gcpCloudProvider) addPlatformDetails(o *Options, cd *hivev1.ClusterDeployment) error {
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			ProjectID: o.GCPProjectID,
			Region:    "us-east1",
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		GCP: &hivev1gcp.PlatformSecrets{
			Credentials: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
		},
	}
	return nil
}

func (p *gcpCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-gcp-creds", o.Name)
}
