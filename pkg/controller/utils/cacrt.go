/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
