package utils

import (
	"context"
	"crypto/md5"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadSecretData loads a given secret key and returns it's data as a string.
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

// GetHashOfPullSecret returns md5 sum of a string
func GetHashOfPullSecret(data string) string {
	bData := []byte(data)
	return fmt.Sprintf("%x", md5.Sum(bData))
}
