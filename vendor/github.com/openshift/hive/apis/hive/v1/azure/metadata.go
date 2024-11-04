package azure

// Metadata contains Azure metadata (e.g. for uninstalling the cluster).
type Metadata struct {
	// ResourceGroupName is the name of the resource group in which the cluster resources were created.
	ResourceGroupName *string `json:"resourceGroupName"`
	// BaseDomainResourceGroupName is the name of the resource group in which the cluster's DNS records were created.
	BaseDomainResourceGroupName *string `json:"baseDomainResourceGroupName"`
}
