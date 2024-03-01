/*
Copyright 2022 Red Hat, Inc.

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

package providerconfig

import (
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"k8s.io/apimachinery/pkg/runtime"
)

// GenericProviderConfig holds the provider spec for machine on platforms that
// don't support failure domains and can be handled generically.
type GenericProviderConfig struct {
	// We don't know the exact type of the providerSpec, but we need to store it to be able to tell if two specs are identical.
	providerSpec *runtime.RawExtension
}

// ExtractFailureDomain extract generic failure domain that contains just the platform type.
func (g GenericProviderConfig) ExtractFailureDomain() failuredomain.FailureDomain {
	return failuredomain.NewGenericFailureDomain()
}

// newGenericProviderConfig creates a generic ProviderConfig that can contain providerSpec for any platform.
func newGenericProviderConfig(providerSpec *runtime.RawExtension, platform configv1.PlatformType) (ProviderConfig, error) {
	config := providerConfig{
		platformType: platform,
		generic: GenericProviderConfig{
			providerSpec: providerSpec,
		},
	}

	return config, nil
}
