package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	additionalCAData []byte
)

// SetupAdditionalCA reads a file referenced by the ADDITIONAL_CA environment
// variable that contains an additional CA. This should only be called once
// on initialization
func SetupAdditionalCA() error {
	additionalCA := os.Getenv("ADDITIONAL_CA")
	if len(additionalCA) == 0 {
		return nil
	}

	data, err := os.ReadFile(additionalCA)
	if err != nil {
		return fmt.Errorf("cannot read additional CA file(%s): %v", additionalCA, err)
	}
	additionalCAData = data
	return nil
}

// AddAdditionalKubeconfigCAs adds additional certificate authorities to a given kubeconfig
func AddAdditionalKubeconfigCAs(data []byte) ([]byte, error) {
	if len(additionalCAData) == 0 {
		return data, nil
	}
	cfg, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}
	for _, cluster := range cfg.Clusters {
		if len(cluster.CertificateAuthorityData) > 0 {
			b := &bytes.Buffer{}
			b.Write(cluster.CertificateAuthorityData)
			b.Write(additionalCAData)
			cluster.CertificateAuthorityData = b.Bytes()
		}
	}
	return clientcmd.Write(*cfg)
}

// TrustBundleFromSecretToWriter creates a trust bundle from keys in the secret writing it to a writer. It assumes all the keys
// have trust bundles in PEM format and writes each one to writer with a newline between each key contents.
func TrustBundleFromSecretToWriter(c client.Client, secretNamespace, secretName string, w io.Writer) error {
	s := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, s)
	if err != nil {
		return err
	}

	for _, v := range s.Data {
		if _, err := w.Write(v); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}
