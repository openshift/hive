package utils

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift/hive/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadSecretData loads a given secret key and returns its data as a string.
func LoadSecretData(c client.Client, secretName, namespace, dataKey string) (string, error) {
	s := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

// Generate the name of the Secret containing AWS Service Provider configuration. The
// prefix should be specified when the env var is known to refer to the global (hive
// namespace) Secret to convert it to the local (CD-specific) name.
// Beware of recursion: we're overloading the env var to refer to both the secret in the
// hive namespace and that in the CD-specific namespace.
func AWSServiceProviderSecretName(prefix string) string {
	spSecretName := os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar)
	// If no secret is given via env, this is n/a
	if spSecretName == "" {
		return ""
	}
	if prefix == "" {
		return spSecretName
	}
	return prefix + "-" + spSecretName
}
