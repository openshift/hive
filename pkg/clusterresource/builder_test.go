package clusterresource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/installer/pkg/types/nutanix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	clusterName                      = "mycluster"
	baseDomain                       = "example.com"
	namespace                        = "mynamespace"
	workerNodeCount                  = 3
	pullSecret                       = "fakepullsecret"
	deleteAfter                      = "8h"
	imageSetName                     = "fake-image-set"
	sshPrivateKey                    = "fakeprivatekey"
	sshPublicKey                     = "fakepublickey"
	fakeManifestFile                 = "my.yaml"
	fakeManifestFileContents         = "fakemanifest"
	fakeAWSAccessKeyID               = "fakeaccesskeyid"
	fakeAWSSecretAccessKey           = "fakesecretAccessKey"
	fakeAzureServicePrincipal        = "fakeSP"
	fakeAzureBaseDomainResourceGroup = "azure-resource-group"
	fakeAzureCloudName               = hivev1azure.CloudEnvironment("AzureUSGovernmentCloud")
	fakeGCPServiceAccount            = "fakeSA"
	fakeGCPProjectID                 = "gcp-project-id"
	adoptAdminKubeconfig             = "adopted-admin-kubeconfig"
	adoptClusterID                   = "adopted-cluster-id"
	adoptInfraID                     = "adopted-infra-id"
	machineNetwork                   = "10.0.0.0/16"
	fakeOpenStackCloudsYAML          = "fakeYAML"
	fakeInstallConfigYaml            = `apiVersion: v1
baseDomain: template.domain
metadata:
  name: template-cluster-name
  creationTimestamp: null
compute:
- hyperthreading: Enabled
  name: worker
controlPlane:
  hyperthreading: Enabled
  name: master
  platform:
    aws:
      type: m5.xlarge
      rootVolume:
        size: 128
      zones:
      - eu-west-1a
      - eu-west-1b
      - eu-west-1c
  replicas: 3
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
`
	fakeMinimalInstallConfigYaml = `apiVersion: v1
compute:
- hyperthreading: Enabled
  name: worker
platform:
  aws:
    region: us-west-2
`
)

func createTestBuilder() *Builder {
	b := &Builder{
		Name:             clusterName,
		Namespace:        namespace,
		WorkerNodesCount: workerNodeCount,
		PullSecret:       pullSecret,
		SSHPrivateKey:    sshPrivateKey,
		SSHPublicKey:     sshPublicKey,
		BaseDomain:       baseDomain,
		Labels: map[string]string{
			"foo": "bar",
		},
		Annotations: map[string]string{},
		InstallerManifests: map[string][]byte{
			fakeManifestFile: []byte(fakeManifestFileContents),
		},
		DeleteAfter:    deleteAfter,
		ImageSet:       imageSetName,
		MachineNetwork: machineNetwork,
	}
	return b
}

func createAWSClusterBuilder() *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &AWSCloudBuilder{
		AccessKeyID:     fakeAWSAccessKeyID,
		SecretAccessKey: fakeAWSSecretAccessKey,
		InstanceType:    AWSInstanceTypeDefault,
	}
	return b
}

func createAzureClusterBuilder() *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &AzureCloudBuilder{
		ServicePrincipal:            []byte(fakeAzureServicePrincipal),
		BaseDomainResourceGroupName: fakeAzureBaseDomainResourceGroup,
		CloudName:                   fakeAzureCloudName,
	}
	return b
}

func createGCPClusterBuilder() *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &GCPCloudBuilder{
		ServiceAccount: []byte(fakeGCPServiceAccount),
		ProjectID:      fakeGCPProjectID,
	}
	return b
}

func createOpenStackClusterBuilder() *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &OpenStackCloudBuilder{
		CloudsYAMLContent: []byte(fakeOpenStackCloudsYAML),
	}
	return b
}

func createVSphereClusterBuilder() *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &VSphereCloudBuilder{
		VCenter:          "test",
		Username:         "test",
		Password:         "test",
		Datacenter:       "test",
		DefaultDatastore: "test",
		Folder:           "test",
		Cluster:          "test",
		APIVIP:           "192.168.0.2",
		IngressVIP:       "192.168.0.3",
		CACert:           []byte{},
	}
	return b
}

func createNutanixClusterBuilder(cert string) *Builder {
	b := createTestBuilder()
	b.CloudBuilder = &NutanixCloudBuilder{
		PrismCentral: nutanix.PrismCentral{
			Endpoint: nutanix.PrismEndpoint{
				Address: "prism-central.nutanix-test.com",
				Port:    9440,
			},
			Username: "nutanix-user",
			Password: "nutanix-password",
		},
		FailureDomains: []nutanix.FailureDomain{
			{
				Name: "fd-name",
				PrismElement: nutanix.PrismElement{
					UUID: "0005de05-75a3-dacb-ba00-2c5da2ac4c1a",
					Endpoint: nutanix.PrismEndpoint{
						Address: "prism-element.nutanix-test.com",
						Port:    9440,
					},
					Name: "PE-NAME",
				},
				SubnetUUIDs:       []string{"0005de05-75a3-dacb-ba00-123456789012"},
				StorageContainers: nil,
				DataSourceImages:  nil,
			},
		},
		APIVIP:     "192.168.0.2",
		IngressVIP: "192.168.0.3",
		CACert:     []byte(cert),
	}
	return b
}

func TestBuildClusterResources(t *testing.T) {
	tests := []struct {
		name     string
		builder  *Builder
		validate func(t *testing.T, allObjects []runtime.Object)
	}{
		{
			name:    "AWS cluster",
			builder: createAWSClusterBuilder(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)
				workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))

				credsSecretName := fmt.Sprintf("%s-aws-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret)
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.AWS.CredentialsSecretRef.Name)

				assert.Equal(t, AWSInstanceTypeDefault, workerPool.Spec.Platform.AWS.InstanceType)
			},
		},
		{
			name: "adopt AWS cluster",
			builder: func() *Builder {
				awsBuilder := createAWSClusterBuilder()
				awsBuilder.Adopt = true
				awsBuilder.AdoptInfraID = adoptInfraID
				awsBuilder.AdoptClusterID = adoptClusterID
				awsBuilder.AdoptAdminKubeconfig = []byte(adoptAdminKubeconfig)
				return awsBuilder
			}(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)

				assert.Equal(t, true, cd.Spec.Installed)
				assert.Equal(t, adoptInfraID, cd.Spec.ClusterMetadata.InfraID)
				assert.Equal(t, adoptClusterID, cd.Spec.ClusterMetadata.ClusterID)

				adminKubeconfig := findSecret(allObjects, fmt.Sprintf("%s-adopted-admin-kubeconfig", clusterName))
				require.NotNil(t, adminKubeconfig)
				assert.Equal(t, adminKubeconfig.Name, cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name)
			},
		},
		{
			name:    "Azure cluster",
			builder: createAzureClusterBuilder(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)
				workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))

				credsSecretName := fmt.Sprintf("%s-azure-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret)
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.Azure.CredentialsSecretRef.Name)

				assert.Equal(t, azureInstanceType, workerPool.Spec.Platform.Azure.InstanceType)

				assert.Equal(t, fakeAzureCloudName, cd.Spec.Platform.Azure.CloudName)
			},
		},
		{
			name:    "GCP cluster",
			builder: createGCPClusterBuilder(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)
				workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))

				credsSecretName := fmt.Sprintf("%s-gcp-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret)
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.GCP.CredentialsSecretRef.Name)

				assert.Equal(t, gcpInstanceType, workerPool.Spec.Platform.GCP.InstanceType)
			},
		},
		{
			name:    "OpenStack cluster",
			builder: createOpenStackClusterBuilder(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)

				credsSecretName := fmt.Sprintf("%s-openstack-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret)
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name)
			},
		},
		{
			name:    "vSphere cluster",
			builder: createVSphereClusterBuilder(),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)

				credsSecretName := fmt.Sprintf("%s-vsphere-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret)
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.VSphere.CredentialsSecretRef.Name)

				certSecretName := fmt.Sprintf("%s-vsphere-certs", clusterName)
				certSecret := findSecret(allObjects, certSecretName)
				require.NotNil(t, certSecret)
				assert.Equal(t, certSecret.Name, cd.Spec.Platform.VSphere.CertificatesSecretRef.Name)
			},
		},
		{
			name:    "Nutanix cluster with certificate",
			builder: createNutanixClusterBuilder("dummy certificate"),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)
				workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))

				credsSecretName := fmt.Sprintf("%s-nutanix-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret, "Nutanix credentials secret should be generated")
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.Nutanix.CredentialsSecretRef.Name)

				certSecretName := fmt.Sprintf("%s-nutanix-certs", clusterName)
				certSecret := findSecret(allObjects, certSecretName)
				require.NotNil(t, certSecret, "Nutanix certificate secret should be generated")
				assert.Equal(t, certSecret.Name, cd.Spec.Platform.Nutanix.CertificatesSecretRef.Name)

				// Validate PrismCentral endpoint
				assert.Equal(t, "prism-central.nutanix-test.com", cd.Spec.Platform.Nutanix.PrismCentral.Address)
				assert.Equal(t, int32(9440), cd.Spec.Platform.Nutanix.PrismCentral.Port)

				// Validate Prism Elements
				require.NotNil(t, cd.Spec.Platform.Nutanix)
				assert.Len(t, cd.Spec.Platform.Nutanix.FailureDomains, 1, "Expected 1 Failure Domain")
				assert.Equal(t, "0005de05-75a3-dacb-ba00-2c5da2ac4c1a", cd.Spec.Platform.Nutanix.FailureDomains[0].PrismElement.UUID)

				// Validate machine pool
				assert.NotNil(t, workerPool)
				assert.Equal(t, int64(workerNodeCount), *workerPool.Spec.Replicas)
			},
		},
		{
			name:    "Nutanix cluster no certificate",
			builder: createNutanixClusterBuilder(""),
			validate: func(t *testing.T, allObjects []runtime.Object) {
				cd := findClusterDeployment(allObjects, clusterName)
				workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))

				credsSecretName := fmt.Sprintf("%s-nutanix-creds", clusterName)
				credsSecret := findSecret(allObjects, credsSecretName)
				require.NotNil(t, credsSecret, "Nutanix credentials secret should be generated")
				assert.Equal(t, credsSecret.Name, cd.Spec.Platform.Nutanix.CredentialsSecretRef.Name)

				certSecretName := fmt.Sprintf("%s-nutanix-certs", clusterName)
				certSecret := findSecret(allObjects, certSecretName)
				require.Nil(t, certSecret)
				assert.Equal(t, cd.Spec.Platform.Nutanix.CertificatesSecretRef.Name, "")

				// Validate PrismCentral endpoint
				assert.Equal(t, "prism-central.nutanix-test.com", cd.Spec.Platform.Nutanix.PrismCentral.Address)
				assert.Equal(t, int32(9440), cd.Spec.Platform.Nutanix.PrismCentral.Port)

				// Validate Prism Elements
				require.NotNil(t, cd.Spec.Platform.Nutanix)
				assert.Len(t, cd.Spec.Platform.Nutanix.FailureDomains, 1, "Expected 1 Failure Domain")
				assert.Equal(t, "0005de05-75a3-dacb-ba00-2c5da2ac4c1a", cd.Spec.Platform.Nutanix.FailureDomains[0].PrismElement.UUID)

				// Validate machine pool
				assert.NotNil(t, workerPool)
				assert.Equal(t, int64(workerNodeCount), *workerPool.Spec.Replicas)
			},
		},
		{
			name: "merge InstallConfigTemplate",
			builder: func() *Builder {
				b := createAWSClusterBuilder()

				b.Name = clusterName
				b.BaseDomain = baseDomain
				b.Namespace = namespace

				b.InstallConfigTemplate = fakeInstallConfigYaml
				yaml.Unmarshal([]byte(fakeInstallConfigYaml), b.InstallConfigTemplate)
				return b
			}(),
			validate: func(t *testing.T, allObjects []runtime.Object) {

				installConfigSecret := findSecret(allObjects, fmt.Sprintf("%s-install-config", clusterName))

				re := strings.NewReplacer("template.domain", baseDomain, "template-cluster-name", clusterName)
				updatedYaml := re.Replace(fakeInstallConfigYaml)

				assert.YAMLEq(t, updatedYaml, installConfigSecret.StringData["install-config.yaml"])
			},
		},
		{
			name: "merge MinimalInstallConfigTemplate",
			builder: func() *Builder {
				b := createAWSClusterBuilder()

				b.Name = clusterName
				b.BaseDomain = baseDomain
				b.Namespace = namespace

				b.InstallConfigTemplate = fakeMinimalInstallConfigYaml
				yaml.Unmarshal([]byte(fakeMinimalInstallConfigYaml), b.InstallConfigTemplate)
				return b
			}(),
			validate: func(t *testing.T, allObjects []runtime.Object) {

				installConfigSecret := findSecret(allObjects, fmt.Sprintf("%s-install-config", clusterName))

				updatedYaml := fakeMinimalInstallConfigYaml + fmt.Sprintf(`baseDomain: %s
metadata:
  creationTimestamp: null
  name: %s`, baseDomain, clusterName)

				assert.YAMLEq(t, updatedYaml, installConfigSecret.StringData["install-config.yaml"])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, test.builder.Validate())
			allObjects, err := test.builder.Build()
			assert.NoError(t, err)

			cd := findClusterDeployment(allObjects, clusterName)
			require.NotNil(t, cd)

			assert.Equal(t, clusterName, cd.Name)
			assert.Equal(t, "bar", cd.Labels["foo"])
			assert.Equal(t, baseDomain, cd.Spec.BaseDomain)
			assert.Equal(t, deleteAfter, cd.Annotations[deleteAfterAnnotation])
			assert.Equal(t, imageSetName, cd.Spec.Provisioning.ImageSetRef.Name)

			installConfigSecret := findSecret(allObjects, fmt.Sprintf("%s-install-config", clusterName))
			require.NotNil(t, installConfigSecret)
			assert.Equal(t, installConfigSecret.Name, cd.Spec.Provisioning.InstallConfigSecretRef.Name)

			pullSecretSecret := findSecret(allObjects, fmt.Sprintf("%s-pull-secret", clusterName))
			require.NotNil(t, pullSecretSecret)
			assert.Equal(t, pullSecretSecret.Name, cd.Spec.PullSecretRef.Name)

			sshKeySecret := findSecret(allObjects, fmt.Sprintf("%s-ssh-private-key", clusterName))
			require.NotNil(t, sshKeySecret)
			assert.Equal(t, sshKeySecret.Name, cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name)

			workerPool := findMachinePool(allObjects, fmt.Sprintf("%s-%s", clusterName, "worker"))
			require.NotNil(t, workerPool)
			nc := int64(workerNodeCount)
			assert.Equal(t, &nc, workerPool.Spec.Replicas)

			manifestsSecret := findSecret(allObjects, fmt.Sprintf("%s-%s", clusterName, "manifests"))
			require.NotNil(t, manifestsSecret)
			assert.Equal(t, manifestsSecret.Name, cd.Spec.Provisioning.ManifestsSecretRef.Name)

			test.validate(t, allObjects)
		})
	}

}

func findSecret(allObjects []runtime.Object, name string) *corev1.Secret {
	for _, ro := range allObjects {
		obj, ok := ro.(*corev1.Secret)
		if !ok {
			continue
		}
		if obj.Name == name {
			return obj
		}
	}
	return nil
}

func findClusterDeployment(allObjects []runtime.Object, name string) *hivev1.ClusterDeployment {
	for _, ro := range allObjects {
		obj, ok := ro.(*hivev1.ClusterDeployment)
		if !ok {
			continue
		}
		if obj.Name == name {
			return obj
		}
	}
	return nil
}

func findMachinePool(allObjects []runtime.Object, name string) *hivev1.MachinePool {
	for _, ro := range allObjects {
		obj, ok := ro.(*hivev1.MachinePool)
		if !ok {
			continue
		}
		if obj.Name == name {
			return obj
		}
	}
	return nil
}
