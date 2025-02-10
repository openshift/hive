package clusterdeployment

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1vpshere "github.com/openshift/hive/apis/hive/v1/vsphere"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/util/scheme"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const testAWSIC = `apiVersion: v1
metadata:
  creationTimestamp: null
  name: testcluster
baseDomain: example.com
compute:
- name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 120
        type: gp3
      type: m5.xlarge
  replicas: 3
controlPlane:
  name: master
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 120
        type: gp3
      type: m5.xlarge
  replicas: 3
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.0.0.0/16
  serviceNetwork:
  - 172.30.0.0/16
platform:
  aws:
    region: us-east-1
pullSecret: ""
`

const testGCPIC = `
apiVersion: v1
baseDomain: example.com
compute:
- name: worker
  platform:
    gcp:
      osDisk:
        DiskSizeGB: 0
        DiskType: ""
      type: n1-standard-4
  replicas: 3
controlPlane:
  name: master
  platform:
    gcp:
      osDisk:
        DiskSizeGB: 0
        DiskType: ""
      type: n1-standard-4
  replicas: 3
metadata:
  creationTimestamp: null
  name: testcluster-gcp
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.0.0.0/16
  serviceNetwork:
  - 172.30.0.0/16
platform:
  gcp:
    projectID: myproject
    region: us-east1
pullSecret: ""
`

const testAzureIC = `
apiVersion: v1
baseDomain: example.com
compute:
- name: worker
  platform:
    azure:
      osDisk:
        diskSizeGB: 0
        diskType: ""
      type: ""
  replicas: 3
controlPlane:
  name: master
  platform:
    azure:
      osDisk:
        diskSizeGB: 0
        diskType: ""
      type: ""
  replicas: 3
metadata:
  creationTimestamp: null
  name: testcluster-azure
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.0.0.0/16
  serviceNetwork:
  - 172.30.0.0/16
platform:
  azure:
    baseDomainResourceGroupName: rg
    outboundType: ""
    region: centralus
pullSecret: ""
`

const testvSphereIC = `
apiVersion: v1
baseDomain: example.com
compute:
- name: worker
controlPlane:
  name: master
metadata:
  name: testcluster-vsphere
platform:
  vsphere:
    vCenter: 10.0.0.1
pullSecret: ""
`
const testNutanixIC = `
apiVersion: v1
baseDomain: example.com
compute:
- name: worker
controlPlane:
  name: master
metadata:
  name: testcluster-nutanix
platform:
  nutanix:
    prismCentral: 
      endpoint: 
        address: 10.0.0.1
pullSecret: ""
`

func TestInstallConfigValidation(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder("testns", "testcluster", scheme)

	tests := []struct {
		name          string
		ic            string
		cd            *hivev1.ClusterDeployment
		expectedError string
		expectNilIC   bool
	}{
		{
			name: "test aws install config valid",
			cd: cdBuilder.Build(
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1"}),
			),
			ic: testAWSIC,
		},
		{
			name: "test install config no aws platform",
			cd: cdBuilder.Build(
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1"}),
			),
			ic:            testGCPIC,
			expectedError: noAWSPlatformErr,
		},
		{
			name: "test aws install config mismatched regions",
			cd: cdBuilder.Build(
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-west-2"}),
			),
			ic:            testAWSIC,
			expectedError: regionMismatchErr,
		},
		{
			name: "test gcp install config valid",
			cd: cdBuilder.Build(
				testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: "us-east1"}),
			),
			ic: testGCPIC,
		},
		{
			name: "test install config no gcp platform",
			cd: cdBuilder.Build(
				testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: "us-east1"}),
			),
			ic:            testAWSIC,
			expectedError: noGCPPlatformErr,
		},
		{
			name: "test gcp install config mismatched regions",
			cd: cdBuilder.Build(
				testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: "us-west2"}),
			),
			ic:            testGCPIC,
			expectedError: regionMismatchErr,
		},
		{
			name: "test azure install config valid",
			cd: cdBuilder.Build(
				testcd.WithAzurePlatform(&hivev1azure.Platform{Region: "centralus"}),
			),
			ic: testAzureIC,
		},
		{
			name: "test install config no azure platform",
			cd: cdBuilder.Build(
				testcd.WithAzurePlatform(&hivev1azure.Platform{Region: "centralus"}),
			),
			ic:            testAWSIC,
			expectedError: noAzurePlatformErr,
		},
		{
			name: "test azure install config mismatched regions",
			cd: cdBuilder.Build(
				testcd.WithAzurePlatform(&hivev1azure.Platform{Region: "us-west2"}),
			),
			ic:            testAzureIC,
			expectedError: regionMismatchErr,
		},
		{
			name: "test install config no vSphere platform",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Spec.Platform.VSphere = &hivev1vpshere.Platform{
						VCenter: "10.0.0.1",
					}
				},
			),
			ic:            testAWSIC,
			expectedError: novSpherePlatformErr,
		},
		{
			name: "test install config no vSphere credentials",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Spec.Platform.VSphere = &hivev1vpshere.Platform{
						VCenter: "10.0.0.1",
					}
				},
			),
			ic:            testvSphereIC,
			expectedError: missingvSphereCredentialsErr,
		},
		//{
		//	name: "test install config no Nutanix platform",
		//	cd: cdBuilder.Build(
		//		func(cd *hivev1.ClusterDeployment) {
		//			cd.Spec.Platform.Nutanix = &hivev1nutanix.Platform{
		//				Endpoint: "10.0.0.1",
		//			}
		//		},
		//	),
		//	ic:            testAWSIC,
		//	expectedError: noNutanixPlatformErr,
		//},
		//{
		//	name: "test install config no Nutanix credentials",
		//	cd: cdBuilder.Build(
		//		func(cd *hivev1.ClusterDeployment) {
		//			cd.Spec.Platform.Nutanix = &hivev1nutanix.Platform{
		//				Endpoint: "10.0.0.1",
		//			}
		//		},
		//	),
		//	ic:            testNutanixIC,
		//	expectedError: missingNutanixCredentialsErr,
		//},
		{
			name: "un-unmarshallable install-config",
			cd: cdBuilder.Build(
				testcd.WithAzurePlatform(&hivev1azure.Platform{Region: "centralus"}),
			),
			ic:            `this is not yaml`,
			expectedError: "could not unmarshal InstallConfig",
			expectNilIC:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			icSecret := &corev1.Secret{
				Data: map[string][]byte{"install-config.yaml": []byte(test.ic)},
			}
			gotIC, err := ValidateInstallConfig(test.cd, icSecret)
			if test.expectedError == "" {
				assert.NoError(t, err)
			} else {
				if assert.Error(t, err, test.expectedError) {
					assert.Contains(t, err.Error(), test.expectedError)
				}
			}
			if test.expectNilIC {
				assert.Nil(t, gotIC)
			} else {
				// We could validate the contents here, but it's not worth it.
				assert.NotNil(t, gotIC)
			}
		})
	}
}
