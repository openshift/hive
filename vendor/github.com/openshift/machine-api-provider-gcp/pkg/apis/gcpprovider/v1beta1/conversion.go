package v1beta1

import "github.com/openshift/machine-api-provider-gcp/pkg/cloud/gcp/actuators/util"

var (
	// RawExtensionFromProviderSpec marshals GCPMachineProviderConfig into a raw extension type.
	// Deprecated: Use util.RawExtensionFromProviderSpec instead.
	RawExtensionFromProviderSpec = util.RawExtensionFromProviderSpec
	// RawExtensionFromProviderStatus marshals GCPMachineProviderStatus into a raw extension type.
	// Deprecated: Use util.RawExtensionFromProviderStatus instead.
	RawExtensionFromProviderStatus = util.RawExtensionFromProviderStatus
	// ProviderSpecFromRawExtension unmarshals a raw extension into an GCPMachineProviderConfig type.
	// Deprecated: Use util.ProviderSpecFromRawExtension instead.
	ProviderSpecFromRawExtension = util.ProviderSpecFromRawExtension
	// ProviderStatusFromRawExtension unmarshals a raw extension into an GCPMachineProviderStatus type.
	// Deprecated: Use util.ProviderStatusFromRawExtension instead.
	ProviderStatusFromRawExtension = util.ProviderStatusFromRawExtension
)
