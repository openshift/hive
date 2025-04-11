package v1

import configv1 "github.com/openshift/api/config/v1"

// FailureDomainSpec extends VSpherePlatformFailureDomainSpec to add additional features for VCM
type FailureDomainSpec struct {
	configv1.VSpherePlatformFailureDomainSpec `json:",inline"`
	// ShortName a short name to be used by CI and other services that need to limit max length of failure domain name
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=30
	// +kubebuilder:validation:Pattern="^[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?$"
	// +optional
	ShortName string `json:"shortName,omitempty"`
}
