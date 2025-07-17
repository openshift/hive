package machinepool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	"github.com/openshift/hive/apis/hive/v1/vsphere"
	ofake "github.com/openshift/hive/pkg/client/fake"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/machinepool/mock"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testmp "github.com/openshift/hive/pkg/test/machinepool"
	teststatefulset "github.com/openshift/hive/pkg/test/statefulset"
)

const (
	testName            = "foo"
	testNamespace       = "default"
	testClusterID       = "foo-12345-uuid"
	testInfraID         = "foo-12345"
	machineAPINamespace = "openshift-machine-api"
	testAMI             = "ami-totallyfake"
	testRegion          = "test-region"
	testPoolName        = "worker"
	testInstanceType    = "test-instance-type"
	testZone            = "test-zone"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestRemoteMachineSetReconcile(t *testing.T) {

	getPool := func(c client.Client, poolName string) *hivev1.MachinePool {
		pool := &hivev1.MachinePool{}
		if err := c.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: fmt.Sprintf("%s-%s", testName, poolName)}, pool); err != nil {
			return nil
		}
		return pool
	}

	// Utility function to list test MachineSets from the fake client
	getRMSL := func(rc client.Client) (*machineapi.MachineSetList, error) {
		rMSL := &machineapi.MachineSetList{}
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
		err := rc.List(context.TODO(), rMSL, &client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}})
		if err == nil {
			return rMSL, err
		}
		return nil, err
	}

	// Utility function to list test MachineAutoscalers from the fake client
	getRMAL := func(rc client.Client) (*autoscalingv1beta1.MachineAutoscalerList, error) {
		rMAL := &autoscalingv1beta1.MachineAutoscalerList{}
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(autoscalingv1beta1.SchemeGroupVersion.WithKind("MachineAutoscaler"))
		err := rc.List(context.TODO(), rMAL, &client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}})
		if err == nil {
			return rMAL, err
		}
		return nil, err
	}

	// Utility function to list test ClusterAutoscalers from the fake client
	getRCAL := func(rc client.Client) (*autoscalingv1.ClusterAutoscalerList, error) {
		rCAL := &autoscalingv1.ClusterAutoscalerList{}
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(autoscalingv1.SchemeGroupVersion.WithKind("ClusterAutoscaler"))
		err := rc.List(context.TODO(), rCAL, &client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}})
		if err == nil {
			return rCAL, err
		}
		return nil, err
	}

	// errStatus describes an expected "False" status condition
	type errStatus struct {
		condType     hivev1.ConditionType
		expReason    string
		msgSubstring string
	}

	tests := []struct {
		name                   string
		clusterDeployment      *hivev1.ClusterDeployment
		machinePool            *hivev1.MachinePool
		remoteExisting         []runtime.Object
		configureRemoteClient  func(*ofake.FakeClientWithCustomErrors)
		generatedMachineSets   []*machineapi.MachineSet
		generateMachineSetsErr error
		actuatorDoNotProceed   bool
		expectErrStatus        *errStatus
		expectNoFinalizer      bool
		// expectPoolPresent is ignored if expectNoFinalizer is false
		expectPoolPresent                bool
		expectedRemoteMachineSets        []*machineapi.MachineSet
		expectedRemoteMachineAutoscalers []autoscalingv1beta1.MachineAutoscaler
		expectedRemoteClusterAutoscalers []autoscalingv1.ClusterAutoscaler
	}{
		{
			name: "Cluster not installed yet",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Installed = false
				return cd
			}(),
			machinePool: testMachinePool(),
		},
		{
			name:              "No-op",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:                 "No-op when actuator says not to proceed",
			clusterDeployment:    testClusterDeployment(),
			machinePool:          testMachinePool(),
			actuatorDoNotProceed: true,
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Update machine set replicas",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 0, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 1, "us-east-1c"),
			},
		},
		{
			name:              "Update machine set ProviderSpec",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(testmp.WithAnnotations(
				map[string]string{constants.OverrideMachinePoolPlatformAnnotation: "true"},
			)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0, replaceProviderSpec(
					func() *machineapi.AWSMachineProviderConfig {
						pc := testAWSProviderSpec()
						pc.Placement.AvailabilityZone = "us-east-1a"
						pc.AMI.ID = aws.String("ami-different")
						return pc
					}(),
				)),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 1, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Update machine set ProviderSpec -- ignored lacking annotation",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0, replaceProviderSpec(
					func() *machineapi.AWSMachineProviderConfig {
						pc := testAWSProviderSpec()
						pc.Placement.AvailabilityZone = "us-east-1a"
						pc.AMI.ID = aws.String("ami-different")
						return pc
					}(),
				)),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0, replaceProviderSpec(
					func() *machineapi.AWSMachineProviderConfig {
						pc := testAWSProviderSpec()
						pc.Placement.AvailabilityZone = "us-east-1a"
						pc.AMI.ID = aws.String("ami-different")
						return pc
					}(),
				)),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Create missing machine set",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Match and update missing MachineSet with different name but matching MachinePool and AZ",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("bar-12345-worker-us-east-1a", "worker", false, 2, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 2, 1, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Error in GenerateMachineSets",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generateMachineSetsErr: fmt.Errorf("foo bar baz"),
			expectErrStatus: &errStatus{
				condType:     hivev1.MachineSetsGeneratedMachinePoolCondition,
				expReason:    "MachineSetGenerationFailed",
				msgSubstring: "foo bar baz",
			},
		},
		{
			name:              "Fail to update MachineSet with same name and AZ but no MachinePool label",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetNotManaged("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"), // no hive MachinePool label
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
			},
			expectErrStatus: &errStatus{
				condType:     hivev1.SyncedMachinePoolCondition,
				expReason:    "MachineSetSyncFailed",
				msgSubstring: "already exists",
			},
		},
		{
			name:              "Fail to update MachineSet with same name and MachinePool label but different AZ",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 2, 0, "not-us-east-1a"),
			},
			expectErrStatus: &errStatus{
				condType:     hivev1.SyncedMachinePoolCondition,
				expReason:    "MachineSetSyncFailed",
				msgSubstring: "already exists",
			},
		},
		{
			name:              "Merge labels and taints",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithLabels(map[string]string{"test-label-2": "test-value-2"}),
				testmp.WithMachineLabels(map[string]string{"mach-label-2": "mach-value-2"}),
				testmp.WithTaints(corev1.Taint{
					Key:    "test-taint-2",
					Value:  "test-value-2",
					Effect: "NoSchedule",
				}),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Labels["test-label"] = "test-value"
					ms.Spec.Template.Labels["mach-label"] = "mach-value"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 1)
					ms.Spec.Template.Spec.Labels["test-label"] = "test-value"
					ms.Spec.Template.Spec.Labels["test-label-2"] = "test-value-2"
					ms.Spec.Template.Labels["mach-label"] = "mach-value"
					ms.Spec.Template.Labels["mach-label-2"] = "mach-value-2"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint-2",
						Value:  "test-value-2",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name:              "Copy over labels and taints from MachinePool, label map nil on remote MachineSet",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithLabels(map[string]string{
					// Allow empty string as value for labels
					"test-label-2": "",
					"test-label-1": "test-value-1",
				}),
				testmp.WithMachineLabels(map[string]string{
					// Allow empty string as value for labels
					"mach-label-2": "",
					"mach-label-1": "mach-value-1",
				}),
				testmp.WithTaints(corev1.Taint{
					Key:    "test-taint",
					Value:  "test-value",
					Effect: "NoSchedule",
				}),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					// remote MachineSet with empty Spec.Template.Spec.Labels
					msReplicas := int32(1)
					rawAWSProviderSpec, err := encodeAWSMachineProviderSpec(testAWSProviderSpec(), scheme.GetScheme())
					if err != nil {
						log.WithError(err).Fatal("error encoding AWS machine provider spec")
					}
					ms := machineapi.MachineSet{
						TypeMeta: metav1.TypeMeta{
							APIVersion: machineapi.SchemeGroupVersion.String(),
							Kind:       "MachineSet",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:       "foo-12345-worker-us-east-1a",
							Namespace:  machineAPINamespace,
							Generation: int64(0),
							Labels: map[string]string{
								"hive.openshift.io/managed": "true",
								machinePoolNameLabel:        testPoolName,
							},
						},
						Spec: machineapi.MachineSetSpec{
							Replicas: &msReplicas,
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"machine.openshift.io/cluster-api-machineset": "foo-12345-worker-us-east-1a",
									"machine.openshift.io/cluster-api-cluster":    testInfraID,
								},
							},
							Template: machineapi.MachineTemplateSpec{
								ObjectMeta: machineapi.ObjectMeta{
									Labels: map[string]string{
										capiClusterKey:    testInfraID,
										capiMachineSetKey: "foo-12345-worker-us-east-1a",
									},
								},
								Spec: machineapi.MachineSpec{
									ProviderSpec: machineapi.ProviderSpec{
										Value: rawAWSProviderSpec,
									},
								},
							},
						},
					}
					return &ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 1)
					ms.Spec.Template.Spec.ObjectMeta.Labels["test-label-1"] = "test-value-1"
					ms.Spec.Template.Spec.ObjectMeta.Labels["test-label-2"] = ""
					ms.Spec.Template.ObjectMeta.Labels["mach-label-1"] = "mach-value-1"
					ms.Spec.Template.ObjectMeta.Labels["mach-label-2"] = ""
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name:              "Update all entries of taints (account for duplicate entries)",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(testmp.WithTaints(
				corev1.Taint{
					Key:    "test-taint",
					Value:  "new-value",
					Effect: "NoSchedule",
				},
				corev1.Taint{
					Key:    "test-taint",
					Value:  "new-value-won't-be-added",
					Effect: "NoSchedule",
				},
			)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 1)
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "new-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name:              "Delete labels and taints if relevant entries in MachinePool Spec modified",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithLabels(map[string]string{"test-label": "keep-me"}),
				testmp.WithOwnedLabels("test-label-2"),
				testmp.WithMachineLabels(map[string]string{"mach-label": "keep-me-too"}),
				testmp.WithOwnedMachineLabels("mach-label-2"),
				testmp.WithTaints(
					corev1.Taint{
						Key:    "test-taint",
						Value:  "keep-me",
						Effect: "NoSchedule",
					},
					corev1.Taint{
						Key:    "test-taint",
						Value:  "collapse-me",
						Effect: "NoSchedule",
					},
				),
				testmp.WithOwnedTaints(
					hivev1.TaintIdentifier{Key: "test-taint-2", Effect: "NoSchedule"},
				),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Labels["test-label"] = "keep-me"
					ms.Spec.Template.Spec.Labels["test-label-2"] = "remove-me"
					ms.Spec.Template.Spec.Labels["test-label-3"] = "preserve-me"
					ms.Spec.Template.Labels["mach-label"] = "keep-me-too"
					ms.Spec.Template.Labels["mach-label-2"] = "remove-me"
					ms.Spec.Template.Labels["mach-label-3"] = "preserve-me-also"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "keep-me",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint-2",
						Value:  "remove-me",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint-3",
						Value:  "preserve-me",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint-3",
						Value:  "collapse-me",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 1)
					ms.Spec.Template.Spec.Labels["test-label"] = "keep-me"
					ms.Spec.Template.Spec.Labels["test-label-3"] = "preserve-me"
					ms.Spec.Template.Labels["mach-label"] = "keep-me-too"
					ms.Spec.Template.Labels["mach-label-3"] = "preserve-me-also"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "keep-me",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint-3",
						Value:  "preserve-me",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name:              "Don't call update if labels or taints on remote MachineSet already exist",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithLabels(map[string]string{
					"test-label-2": "test-value-2",
					"test-label-1": "test-value-1",
				}),
				testmp.WithMachineLabels(map[string]string{
					"mach-label-2": "mach-value-2",
					"mach-label-1": "mach-value-1",
					// Bonus: ignore conflicts with generated labels
					capiClusterKey:    "bogus-cluster",
					capiMachineSetKey: "bogus-machineset",
				}),
				testmp.WithTaints(corev1.Taint{
					Key:    "test-taint",
					Value:  "test-value",
					Effect: "NoSchedule",
				}),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Labels["test-label-1"] = "test-value-1"
					ms.Spec.Template.Spec.Labels["test-label-2"] = "test-value-2"
					ms.Spec.Template.Labels["mach-label-1"] = "mach-value-1"
					ms.Spec.Template.Labels["mach-label-2"] = "mach-value-2"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Labels["test-label-1"] = "test-value-1"
					ms.Spec.Template.Spec.Labels["test-label-2"] = "test-value-2"
					ms.Spec.Template.Labels["mach-label-1"] = "mach-value-1"
					ms.Spec.Template.Labels["mach-label-2"] = "mach-value-2"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name:              "Collapse of duplicate taints should result in an update",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithLabels(map[string]string{"test-label": "test-value"}),
				testmp.WithMachineLabels(map[string]string{"mach-label": "mach-value"}),
				testmp.WithTaints(corev1.Taint{
					Key:    "test-taint",
					Value:  "test-value",
					Effect: "NoSchedule",
				}),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0)
					ms.Spec.Template.Spec.Labels["test-label"] = "test-value"
					ms.Spec.Template.Labels["mach-label"] = "mach-value"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				func() *machineapi.MachineSet {
					ms := testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 1)
					ms.Spec.Template.Spec.Labels["test-label"] = "test-value"
					ms.Spec.Template.Labels["mach-label"] = "mach-value"
					ms.Spec.Template.Spec.Taints = append(ms.Spec.Template.Spec.Taints, corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					})
					return ms
				}(),
			},
		},
		{
			name: "Skip create missing machine set when clusterDeployment has annotation hive.openshift.io/syncset-pause: true ",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Annotations = map[string]string{}
				cd.Annotations[constants.SyncsetPauseAnnotation] = "true"
				return cd
			}(),
			machinePool: testMachinePool(),
		},
		{
			name: "Skip create missing machine set when cluster is unreachable",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				cond.Status = corev1.ConditionTrue
				return cd
			}(),
			machinePool: testMachinePool(),
		},
		{
			name:              "Delete extra machine set",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1d", "worker", true, 1, 0, "us-east-1d"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Other machinesets ignored",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 3, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 3, 0, "us-east-1b"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 3, 0, "us-east-1a"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 3, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 3, 0, "us-east-1b"),
			},
		},
		{
			name:              "Create additional machinepool machinesets",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-other-us-east-1a", "other", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-other-us-east-1c", "other", true, 1, 0, "us-east-1c"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
				testMachineSetWithAZ("foo-12345-other-us-east-1a", "other", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-other-us-east-1c", "other", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "Delete machinepool machinesets",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.Deleted()),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-other-us-east-1a", "other", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-other-us-east-1c", "other", true, 1, 0, "us-east-1c"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectNoFinalizer: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-other-us-east-1a", "other", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-other-us-east-1b", "other", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-other-us-east-1c", "other", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:        "No cluster deployment",
			machinePool: testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectNoFinalizer: true,
			expectPoolPresent: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name: "Deleted cluster deployment",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				now := metav1.Now()
				cd.DeletionTimestamp = &now
				return cd
			}(),
			machinePool: testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectNoFinalizer: true,
			expectPoolPresent: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
		},
		{
			name:              "MachineAutoscaler sync failed",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(1, 2)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				// Pre-seed enough MachineAutoscalers that we'll try to delete some.
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			configureRemoteClient: func(fakeClient *ofake.FakeClientWithCustomErrors) {
				// ...and when we try to delete one, boom.
				fakeClient.DeleteBehavior = []error{fmt.Errorf("foo bar baz")}
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
			},
			expectErrStatus: &errStatus{
				condType:     hivev1.SyncedMachinePoolCondition,
				expReason:    "MachineAutoscalerSyncFailed",
				msgSubstring: "foo bar baz",
			},
		},
		{
			name:              "ClusterAutoscaler sync failed",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(1, 2)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
			},
			configureRemoteClient: func(fakeClient *ofake.FakeClientWithCustomErrors) {
				// The first List() is for MachineAutoscalers. Fail the second, for ClusterAutoscalers
				fakeClient.ListBehavior = []error{nil, fmt.Errorf("foo bar baz")}
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
			},
			expectErrStatus: &errStatus{
				condType:     hivev1.SyncedMachinePoolCondition,
				expReason:    "ClusterAutoscalerSyncFailed",
				msgSubstring: "foo bar baz",
			},
		},
		{
			name:              "No-op with auto-scaling",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("3"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("3"),
			},
		},
		{
			name:              "Create cluster autoscaler",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Skip configuring cluster autoscaler if it already exists",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				func() runtime.Object {
					a := testClusterAutoscaler("1")
					a.Spec.BalanceSimilarNodeGroups = nil
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: func() []autoscalingv1.ClusterAutoscaler {
				a := testClusterAutoscaler("1")
				a.Spec.BalanceSimilarNodeGroups = nil
				return []autoscalingv1.ClusterAutoscaler{*a}
			}(),
		},
		{
			name:              "Don't update cluster autoscaler when BalanceSimilarNodeGroups was explicitly disabled",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				func() runtime.Object {
					a := testClusterAutoscaler("1")
					a.Spec.BalanceSimilarNodeGroups = pointer.Bool(false)
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: func() []autoscalingv1.ClusterAutoscaler {
				a := testClusterAutoscaler("1")
				a.Spec.BalanceSimilarNodeGroups = pointer.Bool(false)
				return []autoscalingv1.ClusterAutoscaler{*a}
			}(),
		},
		{
			name:              "Don't update cluster autoscaler when BalanceSimilarNodeGroups was already enabled",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Create machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Update machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 2, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "2", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "2", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Delete machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(3, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1d", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Delete remote resources for deleted auto-scaling machinepool",
			clusterDeployment: testClusterDeployment(),
			machinePool: testMachinePool(
				testmp.WithAutoscaling(3, 5),
				testmp.Deleted(),
			),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectNoFinalizer: true,
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Create machine autoscalers with zero minReplicas",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(0, 5)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 0, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 0, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 0, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Create machine autoscalers where maxReplicas < #AZs",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(1, 2)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 1),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 0, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Delete machine autoscalers whose maxReplicas would be zero",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(1, 2)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 1, 0, "us-east-1c"),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 2, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", true, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", true, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", true, 0, 1, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 1),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "2", 0, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name:              "Create machine autoscalers where maxReplicas < #AZs and minReplicas==0",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(testmp.WithAutoscaling(0, 2)),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("foo-12345-worker-us-east-1a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("foo-12345-worker-us-east-1c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 0, 1),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 0, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("1"),
			},
		},
		{
			name: "VSphere: generated without suffix, remate without suffix",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform = hivev1.Platform{
					VSphere: &vsphere.Platform{},
				}
				return cd
			}(),
			machinePool: testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker", "worker", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker", "worker", true, 1, 0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			localExisting := []runtime.Object{
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
			}
			if test.clusterDeployment != nil {
				localExisting = append(localExisting, test.clusterDeployment)
			}
			if test.machinePool != nil {
				localExisting = append(localExisting, test.machinePool)
			}
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(localExisting...).Build()
			infra := &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			}
			remoteFakeClient := ofake.FakeClientWithCustomErrors{
				Client: testfake.NewFakeClientBuilder().WithRuntimeObjects(append(test.remoteExisting, infra)...).Build(),
			}
			if test.configureRemoteClient != nil {
				test.configureRemoteClient(&remoteFakeClient)
			}

			mockCtrl := gomock.NewController(t)

			mockActuator := mock.NewMockActuator(mockCtrl)
			if test.generatedMachineSets != nil || test.generateMachineSetsErr != nil {
				mockActuator.EXPECT().
					GenerateMachineSets(test.clusterDeployment, test.machinePool, gomock.Any()).
					Return(test.generatedMachineSets, !test.actuatorDoNotProceed, test.generateMachineSetsErr)
			}

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(remoteFakeClient, nil).AnyTimes()

			logger := log.WithField("controller", "machinepool")
			controllerExpectations := controllerutils.NewExpectations(logger)

			rcd := &ReconcileMachinePool{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        logger,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				actuatorBuilder: func(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, masterMachine *machineapi.Machine, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error) {
					return mockActuator, nil
				},
				expectations: controllerExpectations,
			}
			res, err := rcd.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      fmt.Sprintf("%s-worker", testName),
					Namespace: testNamespace,
				},
			})

			if assert.NoError(t, err, "unexpected error: %v", err) {
				return
			}

			pool := getPool(fakeClient, "worker")

			if test.expectErrStatus != nil {
				cond := controllerutils.FindCondition(pool.Status.Conditions, test.expectErrStatus.condType)
				assert.Equal(t, corev1.ConditionFalse, cond.Status, "Expected %s condition to be False", test.expectErrStatus.condType)
				assert.Equal(t, test.expectErrStatus.expReason, cond.Reason, "Expected %s condition Reason to be %s", test.expectErrStatus.condType, test.expectErrStatus.expReason)
				assert.Contains(t, cond.Message, test.expectErrStatus.msgSubstring, "Did not find expected substring in condition message")
				assert.Equal(t, defaultErrorRequeueInterval, res.RequeueAfter, "Unexpected requeueAfter")
				// Should not proceed with test validations if we expected an error from the reconcile.
				return
			}

			if test.expectNoFinalizer {
				if test.expectPoolPresent {
					assert.NotNil(t, pool, "missing machinepool (with no finalizer)")
				} else {
					assert.Nil(t, pool, "unexpected machinepool")
				}
			} else {
				assert.NotNil(t, pool, "missing machinepool")
				assert.Contains(t, pool.Finalizers, finalizer, "missing finalizer")
			}

			rMSL, err := getRMSL(remoteFakeClient)
			if assert.NoError(t, err) {
				for _, eMS := range test.expectedRemoteMachineSets {
					found := false
					for _, rMS := range rMSL.Items {
						if eMS.Name == rMS.Name {
							found = true
							assert.Equal(t, *eMS.Spec.Replicas, *rMS.Spec.Replicas, "Replicas for %s", rMS.Name)
							assert.Equal(t, eMS.Generation, rMS.Generation, "Generation for %s", rMS.Name)
							if !reflect.DeepEqual(eMS.ObjectMeta.Labels, rMS.ObjectMeta.Labels) {
								t.Errorf("machineset %v has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Labels, rMS.Labels)
							}
							if !reflect.DeepEqual(eMS.ObjectMeta.Annotations, rMS.ObjectMeta.Annotations) {
								t.Errorf("machineset %v has unexpected annotations:\nexpected: %v\nactual: %v", eMS.Name, eMS.Annotations, rMS.Annotations)
							}
							if !reflect.DeepEqual(eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels) {
								t.Errorf("machineset %v machinespec has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels)
							}
							// Check machine labels.
							// Account for nil vs empty map
							if len(eMS.Spec.Template.Labels) != 0 && len(rMS.Spec.Template.Labels) != 0 {
								if !reflect.DeepEqual(eMS.Spec.Template.Labels, rMS.Spec.Template.Labels) {
									t.Errorf("machineset %v machinespec has unexpected machine labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Labels, rMS.Spec.Template.Labels)
								}
							}
							// Taints are stored as a list, so sort them before comparing.
							if !reflect.DeepEqual(sortedTaints(eMS.Spec.Template.Spec.Taints), sortedTaints(rMS.Spec.Template.Spec.Taints)) {
								t.Errorf("machineset %v has unexpected taints:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Taints, rMS.Spec.Template.Spec.Taints)
							}

							rAWSProviderSpec, _ := decodeAWSMachineProviderSpec(rMS.Spec.Template.Spec.ProviderSpec.Value)
							log.Debugf("remote AWS: %v", printAWSMachineProviderConfig(rAWSProviderSpec))
							assert.NotNil(t, rAWSProviderSpec)

							eAWSProviderSpec, _ := decodeAWSMachineProviderSpec(eMS.Spec.Template.Spec.ProviderSpec.Value)
							log.Debugf("expected AWS: %v", printAWSMachineProviderConfig(eAWSProviderSpec))
							assert.NotNil(t, eAWSProviderSpec)
							assert.Equal(t, eAWSProviderSpec.AMI, rAWSProviderSpec.AMI, "%s AMI does not match", eMS.Name)

						}
					}
					if !found {
						t.Errorf("did not find expected remote machineset: %v", eMS.Name)
					}
				}
				for _, rMS := range rMSL.Items {
					found := false
					for _, eMS := range test.expectedRemoteMachineSets {
						if rMS.Name == eMS.Name {
							found = true
						}
					}
					if !found {
						t.Errorf("found unexpected remote machineset: %v", rMS.Name)
					}
				}
			}

			if rMAL, err := getRMAL(remoteFakeClient); assert.NoError(t, err, "error getting machine autoscalers") {
				assert.ElementsMatch(t, test.expectedRemoteMachineAutoscalers, rMAL.Items, "unexpected remote machine autoscalers")
			}

			if rCAL, err := getRCAL(remoteFakeClient); assert.NoError(t, err, "error getting cluster autoscalers") {
				assert.ElementsMatch(t, test.expectedRemoteClusterAutoscalers, rCAL.Items, "unexpected remote cluster autoscalers")
			}
		})
	}
}

// sortedTaints sorts and returns the provided list of taints based on the ascending order of taint key, effect and value - in that order.
func sortedTaints(taints []corev1.Taint) []corev1.Taint {
	sort.SliceStable(taints, func(i, j int) bool {
		if taints[i].Key != taints[j].Key {
			return taints[i].Key < taints[j].Key
		}
		if taints[i].Effect != taints[j].Effect {
			return taints[i].Effect < taints[j].Effect
		}
		return taints[i].Value < taints[j].Value
	})
	return taints
}

func TestUpdateOwnedLabelsTaints(t *testing.T) {
	tests := []struct {
		name                       string
		machinePool                *hivev1.MachinePool
		generatedMachineLabels     []string
		expectedOwnedLabels        []string
		expectedOwnedMachineLabels []string
		expectedOwnedTaints        []hivev1.TaintIdentifier
	}{
		{
			name: "Carry over labels from spec",
			machinePool: testMachinePoolWithoutLabelsTaints(
				testmp.WithLabels(map[string]string{
					"test-label":              "test-value",
					"a-smaller-sorting-label": "z-bigger-value",
				}),
				testmp.WithMachineLabels(map[string]string{
					"mach-label":              "mach-value",
					"a-smaller-sorting-label": "z-bigger-value",
				}),
			),
			expectedOwnedLabels:        []string{"a-smaller-sorting-label", "test-label"},
			expectedOwnedMachineLabels: []string{"a-smaller-sorting-label", "mach-label"},
		},
		{
			name: "Carry over taints from spec",
			machinePool: testMachinePoolWithoutLabelsTaints(
				testmp.WithTaints(
					corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					},
				),
				testmp.WithTaints(
					corev1.Taint{
						Key:    "another-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					},
				),
			),
			expectedOwnedTaints: []hivev1.TaintIdentifier{
				{
					Key:    "another-taint",
					Effect: "NoSchedule",
				},
				{
					Key:    "test-taint",
					Effect: "NoSchedule",
				},
			},
		},
		{
			name: "Remove duplicate taints",
			machinePool: testMachinePoolWithoutLabelsTaints(
				testmp.WithLabels(map[string]string{"test-label": "test-value"}),
				testmp.WithMachineLabels(map[string]string{"mach-label": "mach-value"}),
				testmp.WithTaints(
					corev1.Taint{
						Key:    "test-taint",
						Value:  "test-value",
						Effect: "NoSchedule",
					},
					corev1.Taint{
						Key:    "test-taint",
						Value:  "Value doesn't matter during comparison",
						Effect: "NoSchedule",
					},
				),
			),
			expectedOwnedLabels:        []string{"test-label"},
			expectedOwnedMachineLabels: []string{"mach-label"},
			expectedOwnedTaints: []hivev1.TaintIdentifier{
				{
					Key:    "test-taint",
					Effect: "NoSchedule",
				},
			},
		},
		{
			name: "Generated machineLabels are not owned",
			machinePool: testMachinePoolWithoutLabelsTaints(
				testmp.WithMachineLabels(map[string]string{
					"b-label":         "b-value",
					"generated-label": "another-value",
					"a-label":         "a-value",
					"generated-also":  "z-value",
				}),
			),
			generatedMachineLabels:     []string{"generated-label", "generated-also"},
			expectedOwnedMachineLabels: []string{"a-label", "b-label"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualMachinePoolStatus := updateOwnedLabelsAndTaints(test.machinePool, sets.New(test.generatedMachineLabels...))
			// Explicitly check the length to ensure there aren't any empty entries
			if assert.Equal(t, len(test.expectedOwnedLabels), len(actualMachinePoolStatus.OwnedLabels)) && len(actualMachinePoolStatus.OwnedLabels) > 0 {
				if !reflect.DeepEqual(test.expectedOwnedLabels, actualMachinePoolStatus.OwnedLabels) {
					t.Errorf("expected labels: %v, actual labels: %v", test.expectedOwnedLabels, actualMachinePoolStatus.OwnedLabels)
				}
			}
			// Explicitly check the length to ensure there aren't any empty entries
			if assert.Equal(t, len(test.expectedOwnedMachineLabels), len(actualMachinePoolStatus.OwnedMachineLabels)) && len(actualMachinePoolStatus.OwnedMachineLabels) > 0 {
				if !reflect.DeepEqual(test.expectedOwnedMachineLabels, actualMachinePoolStatus.OwnedMachineLabels) {
					t.Errorf("expected machine labels: %v, actual labels: %v", test.expectedOwnedMachineLabels, actualMachinePoolStatus.OwnedMachineLabels)
				}
			}
			if assert.Equal(t, len(test.expectedOwnedTaints), len(actualMachinePoolStatus.OwnedTaints)) && len(actualMachinePoolStatus.OwnedTaints) > 0 {
				if !reflect.DeepEqual(test.expectedOwnedTaints, actualMachinePoolStatus.OwnedTaints) {
					t.Errorf("expected taints: %v, actual taints: %v", test.expectedOwnedTaints, actualMachinePoolStatus.OwnedTaints)
				}
			}
		})
	}
}

func Test_summarizeMachinesError(t *testing.T) {
	cases := []struct {
		name     string
		existing []runtime.Object

		reason, message string
	}{{
		name: "no matching machines",
		existing: []runtime.Object{
			testMachine("machine-1", "worker"),
			testMachine("machine-2", "worker"),
			testMachine("machine-3", "worker"),
			testMachineSet(testName, "worker", false, 3, 0),
		},
	}, {
		name: "all matching machines ok",
		existing: []runtime.Object{
			testMachineSetMachine("machine-1", "worker", testName),
			testMachineSetMachine("machine-2", "worker", testName),
			testMachineSetMachine("machine-3", "worker", testName),
			testMachineSet(testName, "worker", false, 3, 0),
		},
	}, {
		name: "one machine failed",
		existing: []runtime.Object{
			testMachineSetMachine("machine-1", "worker", testName),
			func() *machineapi.Machine {
				m := testMachineSetMachine("machine-2", "worker", testName)
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.String("GoneNotComingBack"))
				m.Status.ErrorMessage = pointer.String("The machine is not found")
				return m
			}(),
			testMachineSetMachine("machine-3", "worker", testName),
			testMachineSet(testName, "worker", false, 3, 0),
		},

		reason:  "GoneNotComingBack",
		message: "The machine is not found",
	}, {
		name: "more than one machine failed",
		existing: []runtime.Object{
			testMachineSetMachine("machine-1", "worker", testName),
			func() *machineapi.Machine {
				m := testMachineSetMachine("machine-2", "worker", testName)
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.String("GoneNotComingBack"))
				m.Status.ErrorMessage = pointer.String("The machine is not found")
				return m
			}(),
			func() *machineapi.Machine {
				m := testMachineSetMachine("machine-3", "worker", testName)
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.String("InsufficientResources"))
				m.Status.ErrorMessage = pointer.String("No available quota")
				return m
			}(),
			testMachineSet(testName, "worker", false, 3, 0),
		},
		reason: "MultipleMachinesFailed",
		message: `Machine machine-2 failed (GoneNotComingBack): The machine is not found,
Machine machine-3 failed (InsufficientResources): No available quota,
`,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			ms := &machineapi.MachineSet{}
			err := fake.Get(context.TODO(), types.NamespacedName{Namespace: machineAPINamespace, Name: testName}, ms)
			require.NoError(t, err)

			reason, message := summarizeMachinesError(fake, ms, log.WithField("controller", "machinepool"))
			assert.Equal(t, test.reason, reason)
			assert.Equal(t, test.message, message)
		})
	}
}

func testMachinePoolWithoutLabelsTaints(opts ...testmp.Option) *hivev1.MachinePool {
	return testmp.FullBuilder(testNamespace, testPoolName, testName, scheme.GetScheme()).Build(append(
		[]testmp.Option{
			testmp.WithFinalizer(finalizer),
			testmp.WithReplicas(3),
			testmp.WithAWSInstanceType(testInstanceType),
		}, opts...)...)
}

func testMachinePool(opts ...testmp.Option) *hivev1.MachinePool {
	return testMachinePoolWithoutLabelsTaints(append([]testmp.Option{
		testmp.WithLabels(
			map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": testPoolName,
				"machine.openshift.io/cluster-api-machine-type": testPoolName,
			},
		),
		testmp.WithTaints(
			corev1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: corev1.TaintEffectNoSchedule,
			},
		),
		testmp.WithInitializedStatusConditions(),
		testmp.WithControllerOrdinal(0),
	}, opts...)...)
}

func testAWSProviderSpec() *machineapi.AWSMachineProviderConfig {
	return &machineapi.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AWSMachineProviderConfig",
			APIVersion: machineapi.SchemeGroupVersion.String(),
		},
		AMI: machineapi.AWSResourceReference{
			ID: aws.String(testAMI),
		},
	}
}

func replaceProviderSpec(pc *machineapi.AWSMachineProviderConfig) func(*machineapi.MachineSet) {
	rawAWSProviderSpec, err := encodeAWSMachineProviderSpec(pc, scheme.GetScheme())
	if err != nil {
		log.WithError(err).Fatal("error encoding custom machine provider spec")
	}
	return func(ms *machineapi.MachineSet) {
		ms.Spec.Template.Spec.ProviderSpec.Value = rawAWSProviderSpec
	}
}

func testMachineSpec(machineType string) machineapi.MachineSpec {
	rawAWSProviderSpec, err := encodeAWSMachineProviderSpec(testAWSProviderSpec(), scheme.GetScheme())
	if err != nil {
		log.WithError(err).Fatal("error encoding AWS machine provider spec")
	}
	return machineapi.MachineSpec{
		ObjectMeta: machineapi.ObjectMeta{
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": machineType,
				"machine.openshift.io/cluster-api-machine-type": machineType,
			},
		},
		ProviderSpec: machineapi.ProviderSpec{
			Value: rawAWSProviderSpec,
		},

		Taints: []corev1.Taint{
			{
				Key:    "foo",
				Value:  "bar",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}
}

func testMachine(name string, machineType string) *machineapi.Machine {
	return &machineapi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster": testInfraID,
			},
		},
		Spec: testMachineSpec(machineType),
	}
}

func testMachineSetMachine(name string, machineType string, machineSetName string) *machineapi.Machine {
	m := testMachine(name, machineType)
	m.ObjectMeta.Labels["machine.openshift.io/cluster-api-machineset"] = machineSetName
	return m
}

func testMachineSetWithAZ(name string, machineType string, unstompedAnnotation bool, replicas int, generation int, az string) *machineapi.MachineSet {
	pc := testAWSProviderSpec()
	pc.Placement.AvailabilityZone = az

	return testMachineSet(name, machineType, unstompedAnnotation, replicas, generation, replaceProviderSpec(pc))
}

func testMachineSetNotManaged(name string, machineType string, unstompedAnnotation bool, replicas int, generation int, az string) *machineapi.MachineSet {

	ms := testMachineSetWithAZ(name, machineType, unstompedAnnotation, replicas, generation, az)
	delete(ms.Labels, machinePoolNameLabel) // remove machinePoolNameLabel
	return ms
}

func testMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int, mutators ...func(*machineapi.MachineSet)) *machineapi.MachineSet {
	msReplicas := int32(replicas)
	ms := machineapi.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: machineapi.SchemeGroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels: map[string]string{
				machinePoolNameLabel:                       machineType,
				"machine.openshift.io/cluster-api-cluster": testInfraID,
				constants.HiveManagedLabel:                 "true",
			},
			Generation: int64(generation),
		},
		Spec: machineapi.MachineSetSpec{
			Replicas: &msReplicas,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"machine.openshift.io/cluster-api-machineset": name,
					"machine.openshift.io/cluster-api-cluster":    testInfraID,
				},
			},
			Template: machineapi.MachineTemplateSpec{
				ObjectMeta: machineapi.ObjectMeta{
					Labels: map[string]string{
						"machine.openshift.io/cluster-api-machineset": name,
						"machine.openshift.io/cluster-api-cluster":    testInfraID,
					},
				},
				Spec: testMachineSpec(machineType),
			},
		},
	}
	// Add a pre-existing annotation which we will ensure remains in updated machinesets.
	if unstompedAnnotation {
		ms.Annotations = map[string]string{
			"hive.openshift.io/unstomped": "true",
		}
	}

	for _, mutator := range mutators {
		mutator(&ms)
	}
	return &ms
}

func testMachineAutoscaler(name string, resourceVersion string, min, max int) *autoscalingv1beta1.MachineAutoscaler {
	return &autoscalingv1beta1.MachineAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "openshift-machine-api",
			Name:            name,
			ResourceVersion: resourceVersion,
			Labels: map[string]string{
				machinePoolNameLabel: "worker",
			},
		},
		Spec: autoscalingv1beta1.MachineAutoscalerSpec{
			MinReplicas: int32(min),
			MaxReplicas: int32(max),
			ScaleTargetRef: autoscalingv1beta1.CrossVersionObjectReference{
				APIVersion: machineapi.SchemeGroupVersion.String(),
				Kind:       "MachineSet",
				Name:       name,
			},
		},
	}
}

func testClusterAutoscaler(resourceVersion string) *autoscalingv1.ClusterAutoscaler {
	return &autoscalingv1.ClusterAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "default",
			ResourceVersion: resourceVersion,
		},
		Spec: autoscalingv1.ClusterAutoscalerSpec{
			ScaleDown: &autoscalingv1.ScaleDownConfig{
				Enabled: true,
			},
			BalanceSimilarNodeGroups: pointer.Bool(true),
		},
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "ClusterDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
			Labels: map[string]string{
				constants.VersionLabel: "4.4.0",
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testName,
			Platform: hivev1.Platform{
				AWS: &hivev1aws.Platform{
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
					Region: testRegion,
					UserTags: map[string]string{
						"cd-label":              "cd-value",
						"cd-label-sync-from-mp": "cd-value-2",
					},
				},
			},
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-admin-kubeconfig", testName)},
				ClusterID:                testClusterID,
				InfraID:                  testInfraID,
			},
			Installed: true,
		},
		Status: hivev1.ClusterDeploymentStatus{
			Conditions: []hivev1.ClusterDeploymentCondition{{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}},
		},
	}
}

func printAWSMachineProviderConfig(cfg *machineapi.AWSMachineProviderConfig) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		// Where this is used, may as well print the error :shrug:
		return err.Error()
	}
	return string(b)
}

func withClusterVersion(cd *hivev1.ClusterDeployment, version string) *hivev1.ClusterDeployment {
	if cd.Labels == nil {
		cd.Labels = make(map[string]string, 1)
	}
	cd.Labels[constants.VersionLabel] = version
	return cd
}

func TestEnsureEnoughReplicas_ConditionClearing(t *testing.T) {
	tests := []struct {
		name                   string
		pool                   *hivev1.MachinePool
		existingConditions     []hivev1.MachinePoolCondition
		generatedMachineSets   []*machineapi.MachineSet
		expectedConditionState corev1.ConditionStatus
		expectedReason         string
		expectedMessage        string
	}{
		{
			name: "Reset NotEnoughReplicas condition when switching from autoscaling to fixed replicas",
			pool: func() *hivev1.MachinePool {
				mp := testMachinePool(testmp.WithReplicas(3)) // Fixed replicas, no autoscaling
				return mp
			}(),
			existingConditions: []hivev1.MachinePoolCondition{
				{
					Type:    hivev1.NotEnoughReplicasMachinePoolCondition,
					Status:  corev1.ConditionTrue,
					Reason:  "MinReplicasTooSmall",
					Message: "When auto-scaling, the MachinePool must have at least one replica for each MachineSet. The minReplicas must be at least 3",
				},
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("test-worker-a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("test-worker-b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("test-worker-c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedConditionState: corev1.ConditionUnknown,
			expectedReason:         hivev1.InitializedConditionReason,
			expectedMessage:        "Condition Initialized",
		},
		{
			name: "Set NotEnoughReplicas condition true for autoscaling with insufficient minReplicas",
			pool: func() *hivev1.MachinePool {
				mp := testMachinePool(testmp.WithAutoscaling(1, 5)) // Autoscaling with minReplicas < 3 failure domains
				// Use Nutanix platform which doesn't allow zero autoscaling min replicas
				mp.Spec.Platform = hivev1.MachinePoolPlatform{
					Nutanix: &hivev1nutanix.MachinePool{
						FailureDomains: []string{"fd1", "fd2", "fd3"},
					},
				}
				return mp
			}(),
			existingConditions: []hivev1.MachinePoolCondition{},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("test-worker-a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("test-worker-b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("test-worker-c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedConditionState: corev1.ConditionTrue,
			expectedReason:         "MinReplicasTooSmall",
			expectedMessage:        "When auto-scaling, the MachinePool must have at least one replica for each MachineSet. The minReplicas must be at least 3",
		},
		{
			name: "Clear NotEnoughReplicas condition for autoscaling with sufficient minReplicas",
			pool: func() *hivev1.MachinePool {
				mp := testMachinePool(testmp.WithAutoscaling(3, 10)) // Autoscaling with minReplicas >= 3 failure domains
				return mp
			}(),
			existingConditions: []hivev1.MachinePoolCondition{
				{
					Type:    hivev1.NotEnoughReplicasMachinePoolCondition,
					Status:  corev1.ConditionTrue,
					Reason:  "MinReplicasTooSmall",
					Message: "When auto-scaling, the MachinePool must have at least one replica for each MachineSet. The minReplicas must be at least 3",
				},
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("test-worker-a", "worker", false, 1, 0, "us-east-1a"),
				testMachineSetWithAZ("test-worker-b", "worker", false, 1, 0, "us-east-1b"),
				testMachineSetWithAZ("test-worker-c", "worker", false, 1, 0, "us-east-1c"),
			},
			expectedConditionState: corev1.ConditionFalse,
			expectedReason:         "EnoughReplicas",
			expectedMessage:        "The MachinePool has sufficient replicas for each MachineSet",
		},
		{
			name: "Reset NotEnoughReplicas condition when no existing condition",
			pool: func() *hivev1.MachinePool {
				mp := testMachinePool(testmp.WithReplicas(3)) // Fixed replicas, no autoscaling
				return mp
			}(),
			existingConditions: []hivev1.MachinePoolCondition{}, // No existing conditions
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("test-worker-a", "worker", false, 1, 0, "us-east-1a"),
			},
			expectedConditionState: corev1.ConditionUnknown,
			expectedReason:         hivev1.InitializedConditionReason,
			expectedMessage:        "Condition Initialized",
		},
		{
			name: "Allow zero minReplicas for AWS platform autoscaling",
			pool: func() *hivev1.MachinePool {
				mp := testMachinePool(testmp.WithAutoscaling(0, 5)) // Autoscaling with minReplicas = 0
				return mp
			}(),
			existingConditions: []hivev1.MachinePoolCondition{},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSetWithAZ("test-worker-a", "worker", false, 0, 0, "us-east-1a"),
				testMachineSetWithAZ("test-worker-b", "worker", false, 0, 0, "us-east-1b"),
				testMachineSetWithAZ("test-worker-c", "worker", false, 0, 0, "us-east-1c"),
			},
			expectedConditionState: corev1.ConditionFalse,
			expectedReason:         "EnoughReplicas",
			expectedMessage:        "The MachinePool has sufficient replicas for each MachineSet",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			test.pool.Status.Conditions = test.existingConditions

			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.pool).Build()
			cd := testClusterDeployment()
			// Use Nutanix cluster deployment for tests that need platforms that don't allow zero autoscaling min replicas
			if test.name == "Set NotEnoughReplicas condition true for autoscaling with insufficient minReplicas" {
				cd = func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Platform = hivev1.Platform{
						Nutanix: &hivev1nutanix.Platform{},
					}
					return cd
				}()
			}

			logger := log.WithField("controller", "machinepool_test")
			r := &ReconcileMachinePool{
				Client: fakeClient,
				scheme: scheme,
				logger: logger,
			}

			result, err := r.ensureEnoughReplicas(test.pool, test.generatedMachineSets, cd, logger)
			require.NoError(t, err, "ensureEnoughReplicas should not return error")

			// For insufficient replicas case, we expect a non-nil result to stop reconciliation
			if test.expectedConditionState == corev1.ConditionTrue && test.expectedReason == "MinReplicasTooSmall" {
				assert.NotNil(t, result, "ensureEnoughReplicas should return non-nil result for insufficient replicas")
			} else {
				assert.Nil(t, result, "ensureEnoughReplicas should return nil result for sufficient replicas")
			}

			// Refresh the pool to get updated conditions
			updatedPool := &hivev1.MachinePool{}
			err = fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(test.pool), updatedPool)
			require.NoError(t, err, "should be able to get updated pool")

			// Verify the condition state
			cond := controllerutils.FindCondition(updatedPool.Status.Conditions, hivev1.NotEnoughReplicasMachinePoolCondition)
			if assert.NotNil(t, cond, "NotEnoughReplicas condition should exist") {
				assert.Equal(t, test.expectedConditionState, cond.Status, "condition status should match expected")
				assert.Equal(t, test.expectedReason, cond.Reason, "condition reason should match expected")
				assert.Equal(t, test.expectedMessage, cond.Message, "condition message should match expected")
			}
		})
	}
}
