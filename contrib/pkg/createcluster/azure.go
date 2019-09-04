package createcluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	azureCredFile = "osServicePrincipal.json"
)

var _ cloudProvider = (*azureCloudProvider)(nil)

type azureCloudProvider struct {
}

func (acp *azureCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	spFile := filepath.Join(os.Getenv("HOME"), ".azure", azureCredFile)
	log.Info("Loading Azure service principal from: %s", spFile)
	spFileContents, err := ioutil.ReadFile(spFile)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-azure-creds", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			azureCredFile: spFileContents,
		},
	}, nil
}
