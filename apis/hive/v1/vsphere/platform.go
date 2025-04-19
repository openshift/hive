package vsphere

import (
	"fmt"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"strings"
)

// Platform stores any global configuration used for vSphere platforms.
type Platform struct {
	// VCenter is the domain name or IP address of the vCenter.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	VCenter string `json:"vCenter"`

	// CredentialsSecretRef refers to a secret that contains the vSphere account access
	// credentials: GOVC_USERNAME, GOVC_PASSWORD fields.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// CertificatesSecretRef refers to a secret that contains the vSphere CA certificates
	// necessary for communicating with the VCenter.
	CertificatesSecretRef corev1.LocalObjectReference `json:"certificatesSecretRef"`

	// Datacenter is the name of the datacenter to use in the vCenter.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	Datacenter string `json:"datacenter"`

	// DefaultDatastore is the default datastore to use for provisioning volumes.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	DefaultDatastore string `json:"defaultDatastore"`

	// Folder is the name of the folder that will be used and/or created for
	// virtual machines.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	Folder string `json:"folder,omitempty"`

	// Cluster is the name of the cluster virtual machines will be cloned into.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	Cluster string `json:"cluster,omitempty"`

	// Network specifies the name of the network to be used by the cluster.
	// Deprecated: Please use Platform.VSphere instead
	// See also: Platform.ConvertDeprecatedFields
	Network string `json:"network,omitempty"`

	// VSphere is the full spec of the vSphere platform.
	VSphere *configv1.VSpherePlatformSpec `json:"vSphere,omitempty"`
}

func (p *Platform) ConvertDeprecatedFields(logger logr.Logger) {
	if p.VSphere != nil {
		return
	}

	p.VSphere = &configv1.VSpherePlatformSpec{
		VCenters: []configv1.VSpherePlatformVCenterSpec{
			{
				Server:      p.VCenter,
				Port:        443,
				Datacenters: []string{p.Datacenter},
			},
		},
		FailureDomains: []configv1.VSpherePlatformFailureDomainSpec{
			{
				// names from https://github.com/openshift/installer/blob/f7731922a0f17a8339a3e837f72898ac77643611/pkg/types/vsphere/conversion/installconfig.go#L58-L61
				Name:   "generated-failure-domain",
				Region: "generated-region",
				Zone:   "generated-zone",
				Server: p.VCenter,
				Topology: configv1.VSpherePlatformTopology{
					Datacenter:     p.Datacenter,
					Datastore:      setDatastorePath(p.DefaultDatastore, p.Datacenter, logger),
					Folder:         setFolderPath(p.Folder, p.Datacenter, logger),
					ComputeCluster: setComputeClusterPath(p.Cluster, p.Datacenter, logger),
					Networks:       []string{p.Network},
				},
			},
		},
	}

}

// Copied (and slightly modified) from https://github.com/openshift/installer/blob/f7731922a0f17a8339a3e837f72898ac77643611/pkg/types/vsphere/conversion/installconfig.go#L75-L97

func setComputeClusterPath(cluster, datacenter string, logger logr.Logger) string {
	if cluster != "" && !strings.HasPrefix(cluster, "/") {
		logger.V(1).Info(fmt.Sprintf("computeCluster as a non-path is now depreciated please use the form: /%s/host/%s", datacenter, cluster))
		return fmt.Sprintf("/%s/host/%s", datacenter, cluster)
	}
	return cluster
}

func setDatastorePath(datastore, datacenter string, logger logr.Logger) string {
	if datastore != "" && !strings.HasPrefix(datastore, "/") {
		logger.V(1).Info(fmt.Sprintf("datastore as a non-path is now depreciated please use the form: /%s/datastore/%s", datacenter, datastore))
		return fmt.Sprintf("/%s/datastore/%s", datacenter, datastore)
	}
	return datastore
}

func setFolderPath(folder, datacenter string, logger logr.Logger) string {
	if folder != "" && !strings.HasPrefix(folder, "/") {
		logger.V(1).Info(fmt.Sprintf("folder as a non-path is now depreciated please use the form: /%s/vm/%s", datacenter, folder))
		return fmt.Sprintf("/%s/vm/%s", datacenter, folder)
	}
	return folder
}
