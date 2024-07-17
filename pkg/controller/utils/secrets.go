package utils

import (
	"context"
	"errors"
	"fmt"

	"github.com/openshift/hive/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadSecretData loads a given secret key and returns its data as a string.
func LoadSecretData(c client.Client, secretName, namespace, dataKey string) (string, error) {
	s := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retBytes, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	switch dataKey {
	case constants.KubeconfigSecretKey, constants.RawKubeconfigSecretKey:
		if _, err := validateKubeconfig(retBytes); err != nil {
			return "", err
		}
	}
	return string(retBytes), nil
}

// RestConfigFromSecret accepts a Secret containing `kubeconfig` and/or `raw-kubeconfig` keys
// and returns a rest.Config loaded therefrom.
// If tryRaw is true, we will look for `raw-kubeconfig` first and use it if present, falling
// back to `kubeconfig` otherwise.
// The error return is non-nil if:
// - The Secret's Data does not contain the [raw-]kubeconfig key(s)
// - The kubeconfig data cannot be Load()ed
// - The kubeconfig is insecure
func RestConfigFromSecret(kubeconfigSecret *corev1.Secret, tryRaw bool) (*rest.Config, error) {
	var kubeconfigData []byte
	if tryRaw {
		kubeconfigData = kubeconfigSecret.Data[constants.RawKubeconfigSecretKey]
	}
	if len(kubeconfigData) == 0 {
		kubeconfigData = kubeconfigSecret.Data[constants.KubeconfigSecretKey]
	}
	if len(kubeconfigData) == 0 {
		return nil, errors.New("kubeconfig secret does not contain necessary data")
	}
	config, err := validateKubeconfig(kubeconfigData)
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	return kubeConfig.ClientConfig()
}

// validateKubeconfig ensures the kubeconfig represented by kc does not use insecure paths.
// HIVE-2485
func validateKubeconfig(kc []byte) (*api.Config, error) {
	config, err := clientcmd.Load(kc)
	if err != nil {
		return nil, err
	}
	for k, ai := range config.AuthInfos {
		if ai.Exec != nil {
			return nil, fmt.Errorf("insecure exec in AuthInfos[%s]", k)
		}
	}
	return config, nil
}
