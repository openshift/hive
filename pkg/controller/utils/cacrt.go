package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"k8s.io/client-go/tools/clientcmd"
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

	data, err := ioutil.ReadFile(additionalCA)
	if err != nil {
		return fmt.Errorf("cannot read additional CA file(%s): %v", additionalCA, err)
	}
	additionalCAData = data
	return nil
}

// FixupKubeconfig adds additional certificate authorities to a given kubeconfig
func FixupKubeconfig(data []byte) ([]byte, error) {
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
