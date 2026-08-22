package machinepool

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openshift/installer/pkg/types/vsphere"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
)

func TestVSphereActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "deprecated vsphere fields",
			clusterDeployment: testDeprecatedVSphereClusterDeployment(),
			pool:              testVSpherePool(),
			expectedErr:       true,
		},
		{
			name:              "generate machineset",
			clusterDeployment: testVSphereClusterDeployment(),
			pool:              testVSpherePool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker-0", testInfraID): 3,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fds []vsphere.FailureDomain
			var infraID string
			if test.clusterDeployment.Spec.Platform.VSphere != nil &&
				test.clusterDeployment.Spec.Platform.VSphere.Infrastructure != nil {
				fds = test.clusterDeployment.Spec.Platform.VSphere.Infrastructure.FailureDomains
			}
			if test.clusterDeployment.Spec.ClusterMetadata != nil {
				infraID = test.clusterDeployment.Spec.ClusterMetadata.InfraID
			}
			logger := log.WithField("actuator", "vsphereactuator_test")
			actuator, err := NewVSphereActuator(nil, infraID, fds, logger)
			assert.NoError(t, err, "unexpected error creating VSphereActuator")

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test cast")
				validateVSphereMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func TestBackfillVSphereTemplates(t *testing.T) {
	logger := log.WithField("test", "backfill")

	tests := []struct {
		name              string
		failureDomains    []vsphere.FailureDomain
		remoteMachineSets []machineapi.MachineSet
		infraID           string
		expectedTemplates map[string]string
	}{
		{
			name: "non-zonal: backfill from matching MachineSet",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "generated-failure-domain",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "datastore1", "/dc1/host/cluster1/Resources", "", "/dc1/vm/test-infra-rhcos-generated-region-generated-zone"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"generated-failure-domain": "/dc1/vm/test-infra-rhcos-generated-region-generated-zone",
			},
		},
		{
			name: "non-zonal: template already set, not overwritten",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "fd1",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources",
						Template:     "/dc1/vm/explicit-template",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "datastore1", "/dc1/host/cluster1/Resources", "", "/dc1/vm/test-infra-rhcos-fd1"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"fd1": "",
			},
		},
		{
			name: "non-zonal: no matching remote MachineSet, template stays empty",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "fd1",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "other-vcenter.example.com", "dc2", "datastore2", "/dc2/host/cluster2/Resources", "", "/dc2/vm/test-infra-rhcos-fd2"),
			},
			infraID:           "test-infra",
			expectedTemplates: map[string]string{"fd1": ""},
		},
		{
			name: "compute-cluster zonal: ResourcePool disambiguates FDs",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:     "us-east-1",
					Server:   "vcenter.example.com",
					ZoneType: vsphere.ComputeClusterFailureDomain,
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "shared-ds",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
				{
					Name:     "us-east-2",
					Server:   "vcenter.example.com",
					ZoneType: vsphere.ComputeClusterFailureDomain,
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "shared-ds",
						ResourcePool: "/dc1/host/cluster2/Resources",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "shared-ds", "/dc1/host/cluster1/Resources", "", "/dc1/vm/test-infra-rhcos-us-east-1"),
				testRemoteMachineSet("test-infra-worker-1", "vcenter.example.com", "dc1", "shared-ds", "/dc1/host/cluster2/Resources", "", "/dc1/vm/test-infra-rhcos-us-east-2"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"us-east-1": "/dc1/vm/test-infra-rhcos-us-east-1",
				"us-east-2": "/dc1/vm/test-infra-rhcos-us-east-2",
			},
		},
		{
			name: "host-group zonal: VMGroup disambiguates FDs",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:     "zone-a",
					Server:   "vcenter.example.com",
					ZoneType: vsphere.HostGroupFailureDomain,
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "shared-ds",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
				{
					Name:     "zone-b",
					Server:   "vcenter.example.com",
					ZoneType: vsphere.HostGroupFailureDomain,
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "shared-ds",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "shared-ds", "/dc1/host/cluster1/Resources", "test-infra-zone-a", "/dc1/vm/test-infra-rhcos-zone-a"),
				testRemoteMachineSet("test-infra-worker-1", "vcenter.example.com", "dc1", "shared-ds", "/dc1/host/cluster1/Resources", "test-infra-zone-b", "/dc1/vm/test-infra-rhcos-zone-b"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"zone-a": "/dc1/vm/test-infra-rhcos-zone-a",
				"zone-b": "/dc1/vm/test-infra-rhcos-zone-b",
			},
		},
		{
			name: "no remote MachineSets: template stays empty",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "fd1",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
			},
			remoteMachineSets: nil,
			infraID:           "test-infra",
			expectedTemplates: map[string]string{"fd1": ""},
		},
		{
			name: "ResourcePool normalization: trailing slash match",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "fd1",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources/",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "datastore1", "/dc1/host/cluster1/Resources", "", "/dc1/vm/test-infra-rhcos-fd1"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"fd1": "/dc1/vm/test-infra-rhcos-fd1",
			},
		},
		{
			name: "MachinePool ResourcePool override does not affect template discovery",
			failureDomains: []vsphere.FailureDomain{
				{
					Name:   "fd1",
					Server: "vcenter.example.com",
					Topology: vsphere.Topology{
						Datacenter:   "dc1",
						Datastore:    "datastore1",
						ResourcePool: "/dc1/host/cluster1/Resources",
					},
				},
			},
			remoteMachineSets: []machineapi.MachineSet{
				testRemoteMachineSet("test-infra-worker-0", "vcenter.example.com", "dc1", "datastore1", "/dc1/host/cluster1/Resources", "", "/dc1/vm/test-infra-rhcos-fd1"),
			},
			infraID: "test-infra",
			expectedTemplates: map[string]string{
				"fd1": "/dc1/vm/test-infra-rhcos-fd1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			templates := buildTemplateMap(test.failureDomains, test.remoteMachineSets, test.infraID, logger)

			for fdName, expected := range test.expectedTemplates {
				if expected == "" {
					assert.NotContains(t, templates, fdName, "expected no template for FD %s", fdName)
				} else {
					assert.Equal(t, expected, templates[fdName], "unexpected template for FD %s", fdName)
				}
			}
		})
	}
}

func testRemoteMachineSet(name, server, datacenter, datastore, resourcePool, vmGroup, template string) machineapi.MachineSet {
	spec := &machineapi.VSphereMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VSphereMachineProviderSpec",
			APIVersion: machineapi.SchemeGroupVersion.String(),
		},
		Template: template,
		Workspace: &machineapi.Workspace{
			Server:       server,
			Datacenter:   datacenter,
			Datastore:    datastore,
			ResourcePool: resourcePool,
			VMGroup:      vmGroup,
		},
	}
	raw, _ := json.Marshal(spec)
	return machineapi.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: machineapi.MachineSetSpec{
			Template: machineapi.MachineTemplateSpec{
				Spec: machineapi.MachineSpec{
					ProviderSpec: machineapi.ProviderSpec{
						Value: &runtime.RawExtension{Raw: raw},
					},
				},
			},
		},
	}
}

func validateVSphereMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		vsphereProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.VSphereMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to vsphere provider spec") {
			assert.Equal(t, int64(32*1024), vsphereProvider.MemoryMiB, "unexpected MemeoryMiB")
			assert.Equal(t, int32(4), vsphereProvider.NumCPUs, "unexpected NumCPUs")
			assert.Equal(t, int32(4), vsphereProvider.NumCoresPerSocket, "unexpected NumCoresPerSocket")
			assert.Equal(t, int32(512), vsphereProvider.DiskGiB, "unexpected DiskGiB")
			assert.Equal(t, "default-folder", vsphereProvider.Workspace.Folder, "unexpected Folder")
			assert.Equal(t, "good-pool", vsphereProvider.Workspace.ResourcePool, "unexpected ResourcePool")
			if assert.Len(t, vsphereProvider.TagIDs, 1, "missing tag IDs") {
				assert.Equal(t, vsphereProvider.TagIDs[0], "vsphere-tag")
			}
		}
	}
}

func testVSpherePool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		VSphere: &hivev1vsphere.MachinePool{
			MachinePool: vsphere.MachinePool{
				MemoryMiB:         32 * 1024,
				NumCPUs:           4,
				NumCoresPerSocket: 4,
				OSDisk: vsphere.OSDisk{
					DiskSizeGB: 512,
				},
			},
			ResourcePool: "good-pool",
			TagIDs:       []string{"vsphere-tag"},
		},
	}
	return p
}

func testDeprecatedVSphereClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		VSphere: &hivev1vsphere.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "vsphere-credentials",
			},
			DeprecatedFolder: "/vsphere-datacenter/vm/vsphere-folder",
		},
	}
	return cd
}

func testVSphereClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		VSphere: &hivev1vsphere.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "vsphere-credentials",
			},
			Infrastructure: &vsphere.Platform{
				VCenters: []vsphere.VCenter{
					{
						Server: "test-server",
					},
				},
				FailureDomains: []vsphere.FailureDomain{
					{
						Server: "test-server",
						Topology: vsphere.Topology{
							ResourcePool: "default-pool",
							Folder:       "default-folder",
						},
					},
				},
			},
		},
	}
	return cd
}
