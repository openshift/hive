package creds

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/installer/pkg/types"

	"github.com/openshift/hive/pkg/constants"
	awsutil "github.com/openshift/hive/pkg/creds/aws"
	azurecreds "github.com/openshift/hive/pkg/creds/azure"
	gcpcreds "github.com/openshift/hive/pkg/creds/gcp"
	ibmcloudutil "github.com/openshift/hive/pkg/creds/ibmcloud"
	nutanixutil "github.com/openshift/hive/pkg/creds/nutanix"
	openstackutil "github.com/openshift/hive/pkg/creds/openstack"
	vsphereutil "github.com/openshift/hive/pkg/creds/vsphere"
)

var ConfigureCreds = map[string]func(client.Client, *types.ClusterMetadata){
	constants.PlatformAWS:       awsutil.ConfigureCreds,
	constants.PlatformAzure:     azurecreds.ConfigureCreds,
	constants.PlatformGCP:       gcpcreds.ConfigureCreds,
	constants.PlatformIBMCloud:  ibmcloudutil.ConfigureCreds,
	constants.PlatformNutanix:   nutanixutil.ConfigureCreds,
	constants.PlatformOpenStack: openstackutil.ConfigureCreds,
	constants.PlatformVSphere:   vsphereutil.ConfigureCreds,
}
