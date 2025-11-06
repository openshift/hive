package v1

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	"github.com/openshift/hive/pkg/constants"
)

func Test_MachinePoolAdmission_Validate_Kind(t *testing.T) {
	cases := []struct {
		name         string
		group        string
		version      string
		resource     string
		expectToSkip bool
	}{
		{
			name:     "machinepool",
			group:    machinePoolGroup,
			version:  machinePoolVersion,
			resource: machinePoolResource,
		},
		{
			name:         "different group",
			group:        "other group",
			version:      machinePoolVersion,
			resource:     machinePoolResource,
			expectToSkip: true,
		},
		{
			name:         "different version",
			group:        machinePoolGroup,
			version:      "other version",
			resource:     machinePoolResource,
			expectToSkip: true,
		},
		{
			name:         "different resource",
			group:        machinePoolGroup,
			version:      machinePoolVersion,
			resource:     "other resource",
			expectToSkip: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewMachinePoolValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    tc.group,
					Version:  tc.version,
					Resource: tc.resource,
				},
				Operation: admissionv1beta1.Create,
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectToSkip, response.Allowed)
		})
	}
}

func Test_MachinePoolAdmission_Validate_Operation(t *testing.T) {
	cases := []struct {
		name         string
		operation    admissionv1beta1.Operation
		expectToSkip bool
	}{
		{
			name:      "create",
			operation: admissionv1beta1.Create,
		},
		{
			name:      "update",
			operation: admissionv1beta1.Update,
		},
		{
			name:         "other",
			operation:    admissionv1beta1.Operation("other"),
			expectToSkip: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewMachinePoolValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    machinePoolGroup,
					Version:  machinePoolVersion,
					Resource: machinePoolResource,
				},
				Operation: tc.operation,
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectToSkip, response.Allowed)
		})
	}
}

func Test_MachinePoolAdmission_Validate_Create(t *testing.T) {
	cases := []struct {
		name          string
		provision     *hivev1.MachinePool
		expectAllowed bool
	}{
		{
			name:          "good",
			provision:     testMachinePool(),
			expectAllowed: true,
		},
		{
			name: "missing clusterdeployment ref",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.ClusterDeploymentRef.Name = ""
				return pool
			}(),
		},
		{
			name: "missing remote machine pool name",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Name = ""
				return pool
			}(),
		},
		{
			name: "master pool name",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Name = fmt.Sprintf("%s-%s", pool.Spec.ClusterDeploymentRef.Name, defaultMasterPoolName)
				pool.Spec.Name = defaultMasterPoolName
				return pool
			}(),
		},
		{
			name: "legacy worker pool name",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Name = fmt.Sprintf("%s-%s", pool.Spec.ClusterDeploymentRef.Name, legacyWorkerPoolName)
				pool.Spec.Name = legacyWorkerPoolName
				return pool
			}(),
		},
		{
			name: "invalid name",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Name = ""
				pool.Name = "bad-name"
				return pool
			}(),
		},
		{
			name: "zero replicas",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Replicas = ptr.To(int64(0))
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "positive replicas",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Replicas = ptr.To(int64(1))
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "negative replicas",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Replicas = ptr.To(int64(-1))
				return pool
			}(),
		},
		{
			name: "replicas and autoscaling",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Replicas = ptr.To(int64(1))
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{}
				return pool
			}(),
		},
		{
			name: "zero min replicas",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 0,
					MaxReplicas: 1,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "zero min replicas unsupported",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 0,
					MaxReplicas: 1,
				}

				pool.Spec.Platform.VSphere = validvSphereMachinePoolPlatform()
				return pool
			}(),
		},
		{
			name: "min replicas less than number of AWS zones",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 1,
					MaxReplicas: 1,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas equal to number of AWS zones",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 2,
					MaxReplicas: 2,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas less than number of GCP zones",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Platform.GCP.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 1,
					MaxReplicas: 1,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas equal to number of GCP zones",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Platform.GCP.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 2,
					MaxReplicas: 2,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas less than number of Azure zones",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 1,
					MaxReplicas: 1,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas equal to number of Azure zones",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.Zones = []string{"zone1", "zone2"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 2,
					MaxReplicas: 2,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "min replicas greater than max replicas",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 2,
					MaxReplicas: 1,
				}
				return pool
			}(),
		},
		{
			name: "missing platform",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform = hivev1.MachinePoolPlatform{}
				return pool
			}(),
		},
		{
			name: "multiple platforms",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.GCP = validGCPMachinePoolPlatform()
				return pool
			}(),
		},
		{
			name: "explicit AWS zones",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"test-zone-1", "test-zone-2"}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "empty AWS zone name",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{""}
				return pool
			}(),
		},
		{
			name: "missing AWS instance type",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.InstanceType = ""
				return pool
			}(),
		},
		{
			name: "invalid AWS volume IOPS",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.EC2RootVolume.IOPS = -1
				return pool
			}(),
		},
		{
			name: "invalid AWS volume size",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.EC2RootVolume.Size = -1
				return pool
			}(),
		},
		{
			name: "missing AWS volume type",
			provision: func() *hivev1.MachinePool {
				pool := testAWSMachinePool()
				pool.Spec.Platform.AWS.EC2RootVolume.Type = ""
				return pool
			}(),
		},
		{
			name: "non-default GCP pool",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Name = "other-name"
				pool.Name = fmt.Sprintf("%s-%s", pool.Name, pool.Spec.Name)
				return pool
			}(),
		},
		{
			name: "explicit GCP zones",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Platform.GCP.Zones = []string{"test-zone-1", "test-zone-2"}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "empty GCP zone name",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Platform.GCP.Zones = []string{""}
				return pool
			}(),
		},
		{
			name: "missing GCP instance type",
			provision: func() *hivev1.MachinePool {
				pool := testGCPMachinePool()
				pool.Spec.Platform.GCP.InstanceType = ""
				return pool
			}(),
		},
		{
			name: "explicit Azure zones",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.Zones = []string{"test-zone-1", "test-zone-2"}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "empty Azure zone name",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.Zones = []string{""}
				return pool
			}(),
		},
		{
			name: "missing Azure instance type",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.InstanceType = ""
				return pool
			}(),
		},
		{
			name: "invalid Azure disk size",
			provision: func() *hivev1.MachinePool {
				pool := testAzureMachinePool()
				pool.Spec.Platform.Azure.OSDisk.DiskSizeGB = 0
				return pool
			}(),
		},
		{
			name: "valid labels",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Labels = map[string]string{"test-label-key": "test-label-value"}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "invalid labels",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Labels = map[string]string{".bad-label-key": "test-label-value"}
				return pool
			}(),
		},
		{
			name: "zero autoscaling",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 0,
					MaxReplicas: 0,
				}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "zero autoscaling with defined zones",
			provision: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zoneA", "zoneB"}
				pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 0,
					MaxReplicas: 0,
				}
				return pool
			}(),
			expectAllowed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewMachinePoolValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			rawProvision, err := json.Marshal(tc.provision)
			if !assert.NoError(t, err, "unexpected error marshalling provision") {
				return
			}
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    machinePoolGroup,
					Version:  machinePoolVersion,
					Resource: machinePoolResource,
				},
				Operation: admissionv1beta1.Create,
				Object:    runtime.RawExtension{Raw: rawProvision},
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectAllowed, response.Allowed, "unexpected response: %#v", response.Result)
		})
	}
}

func Test_MachinePoolAdmission_Validate_Update(t *testing.T) {
	cases := []struct {
		name          string
		old           *hivev1.MachinePool
		new           *hivev1.MachinePool
		expectAllowed bool
	}{
		{
			name:          "no changes",
			old:           testMachinePool(),
			new:           testMachinePool(),
			expectAllowed: true,
		},
		{
			name: "cluster deployment ref changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.ClusterDeploymentRef.Name = "new-cluster-deployment"
				return pool
			}(),
		},
		{
			name: "remote machine pool name changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Name = "new-machine-pool"
				return pool
			}(),
		},
		{
			name: "replicas changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Replicas = ptr.To(int64(5))
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "labels changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Labels = map[string]string{"new-label-key": "new-label-value"}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "taints changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Taints = []corev1.Taint{{
					Key:    "new-taint-key",
					Value:  "new-taint-value",
					Effect: corev1.TaintEffectNoSchedule,
				}}
				return pool
			}(),
			expectAllowed: true,
		},
		{
			name: "platform changed",
			old:  testMachinePool(),
			new:  testGCPMachinePool(),
		},
		{
			name: "platform changed with override",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				mp := testGCPMachinePool()
				mp.Annotations = map[string]string{constants.OverrideMachinePoolPlatformAnnotation: "true"}
				return mp
			}(),
			expectAllowed: true,
		},
		{
			name: "instance type changed",
			old:  testMachinePool(),
			new: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.InstanceType = "other-instance-type"
				return pool
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewMachinePoolValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			oldAsJSON, err := json.Marshal(tc.old)
			if !assert.NoError(t, err, "unexpected error marshalling old provision") {
				return
			}
			newAsJSON, err := json.Marshal(tc.new)
			if !assert.NoError(t, err, "unexpected error marshalling new provision") {
				return
			}
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    machinePoolGroup,
					Version:  machinePoolVersion,
					Resource: machinePoolResource,
				},
				Operation: admissionv1beta1.Update,
				Object:    runtime.RawExtension{Raw: newAsJSON},
				OldObject: runtime.RawExtension{Raw: oldAsJSON},
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectAllowed, response.Allowed, "unexpected response: %#v", response.Result)
		})
	}
}

func testMachinePool() *hivev1.MachinePool {
	cdName := "test-deployment"
	return &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", cdName, defaultWorkerPoolName),
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: cdName,
			},
			Name: defaultWorkerPoolName,
			Platform: hivev1.MachinePoolPlatform{
				AWS: validAWSMachinePoolPlatform(),
			},
		},
	}
}

func testAWSMachinePool() *hivev1.MachinePool {
	pool := testMachinePool()
	pool.Spec.Platform = hivev1.MachinePoolPlatform{
		AWS: validAWSMachinePoolPlatform(),
	}
	return pool
}

func testGCPMachinePool() *hivev1.MachinePool {
	pool := testMachinePool()
	pool.Spec.Platform = hivev1.MachinePoolPlatform{
		GCP: validGCPMachinePoolPlatform(),
	}
	return pool
}

func testAzureMachinePool() *hivev1.MachinePool {
	pool := testMachinePool()
	pool.Spec.Platform = hivev1.MachinePoolPlatform{
		Azure: validAzureMachinePoolPlatform(),
	}
	return pool
}

func validAWSMachinePoolPlatform() *hivev1aws.MachinePoolPlatform {
	return &hivev1aws.MachinePoolPlatform{
		InstanceType: "test-instance-type",
		EC2RootVolume: hivev1aws.EC2RootVolume{
			IOPS: 1,
			Size: 2,
			Type: "test-volume-type",
		},
	}
}

func validGCPMachinePoolPlatform() *hivev1gcp.MachinePool {
	return &hivev1gcp.MachinePool{
		InstanceType: "test-instance-type",
	}
}

func validAzureMachinePoolPlatform() *hivev1azure.MachinePool {
	return &hivev1azure.MachinePool{
		InstanceType: "test-instance-type",
		OSDisk: hivev1azure.OSDisk{
			DiskSizeGB: 1,
		},
	}
}

func validvSphereMachinePoolPlatform() *hivev1vsphere.MachinePool {
	return &hivev1vsphere.MachinePool{
		OSDisk: hivev1vsphere.OSDisk{
			DiskSizeGB: 1,
		},
	}
}
