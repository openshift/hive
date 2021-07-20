package machinepool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsproviderapis "sigs.k8s.io/cluster-api-provider-aws/pkg/apis"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/machinepool/mock"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
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
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestRemoteMachineSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)
	awsproviderapis.AddToScheme(scheme.Scheme)

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

	tests := []struct {
		name                     string
		clusterDeployment        *hivev1.ClusterDeployment
		machinePool              *hivev1.MachinePool
		remoteExisting           []runtime.Object
		localExisting            []runtime.Object
		generatedMachineSets     []*machineapi.MachineSet
		generatedCAPIMachineSets []*capiv1.MachineSet
		actuatorDoNotProceed     bool
		expectErr                bool
		expectNoFinalizer        bool
		// expectPoolPresent is ignored if expectNoFinalizer is false
		expectPoolPresent                bool
		expectedRemoteMachineSets        []*machineapi.MachineSet
		expectedRemoteMachineAutoscalers []autoscalingv1beta1.MachineAutoscaler
		expectedRemoteClusterAutoscalers []autoscalingv1.ClusterAutoscaler
		CAPI                             bool
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
		},
		{
			name:                 "No-op when actuator says not to proceed",
			clusterDeployment:    testClusterDeployment(),
			machinePool:          testMachinePool(),
			actuatorDoNotProceed: true,
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
		},
		{
			name:              "Update machine set replicas",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 0, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 1),
			},
		},
		{
			name:              "Create missing machine set",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
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
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1d", "worker", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
		},
		{
			name:              "Other machinesets ignored",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 3, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 3, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 3, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 3, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 3, 0),
			},
		},
		{
			name:              "Create additional machinepool machinesets",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-other-us-east-1a", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1c", "other", true, 1, 0),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
				testMachineSet("foo-12345-other-us-east-1a", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1c", "other", true, 1, 0),
			},
		},
		{
			name:              "Delete machinepool machinesets",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				mp := testMachinePool()
				now := metav1.Now()
				mp.DeletionTimestamp = &now
				return mp
			}(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-other-us-east-1a", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1c", "other", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectNoFinalizer: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-other-us-east-1a", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1b", "other", true, 1, 0),
				testMachineSet("foo-12345-other-us-east-1c", "other", true, 1, 0),
			},
		},
		{
			name:        "No cluster deployment",
			machinePool: testMachinePool(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectNoFinalizer: true,
			expectPoolPresent: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectNoFinalizer: true,
			expectPoolPresent: true,
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
		},
		{
			name:              "No-op with auto-scaling",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler("3"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			name:              "Update cluster autoscaler when missing scale down",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				func() runtime.Object {
					a := testClusterAutoscaler("1")
					a.Spec.ScaleDown = nil
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("2"),
			},
		},
		{
			name:              "Update cluster autoscaler when scale down disabled",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				func() runtime.Object {
					a := testClusterAutoscaler("1")
					a.Spec.ScaleDown.Enabled = false
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectedRemoteMachineAutoscalers: []autoscalingv1beta1.MachineAutoscaler{
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler("2"),
			},
		},
		{
			name:              "Create machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 2, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler("1"),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", "1", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", "1", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1d", "1", 1, 1),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			machinePool: func() *hivev1.MachinePool {
				mp := testAutoscalingMachinePool(3, 5)
				now := metav1.Now()
				mp.DeletionTimestamp = &now
				return mp
			}(),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
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
			machinePool:       testAutoscalingMachinePool(0, 5),
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
				testClusterAutoscaler("1"),
			},
			generatedMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 0, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 0, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 0, 0),
			},
			expectedRemoteMachineSets: []*machineapi.MachineSet{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", false, 0, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", false, 0, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 0, 0),
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
			name: "CMM",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.MachineManagement = &hivev1.MachineManagement{
					Central: &hivev1.CentralMachineManagement{},
				}
				return cd
			}(),
			machinePool: testMachinePool(),
			CAPI:        true,
			remoteExisting: []runtime.Object{
				testMachine("master1", "master"),
			},
			localExisting: []runtime.Object{
				testCAPIMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
			},
			generatedCAPIMachineSets: []*capiv1.MachineSet{
				testCAPIMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testCAPIMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testCAPIMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		capiv1.AddToScheme(scheme.Scheme)
		machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)
		autoscalingv1.SchemeBuilder.AddToScheme(scheme.Scheme)
		autoscalingv1beta1.SchemeBuilder.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			localExisting := test.localExisting
			if test.clusterDeployment != nil {
				localExisting = append(localExisting, test.clusterDeployment)
			}
			if test.machinePool != nil {
				localExisting = append(localExisting, test.machinePool)
			}
			fakeClient := fake.NewClientBuilder().WithRuntimeObjects(localExisting...).Build()
			remoteFakeClient := fake.NewClientBuilder().WithRuntimeObjects(test.remoteExisting...).Build()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockActuator := mock.NewMockActuator(mockCtrl)
			if !test.CAPI && test.generatedMachineSets != nil {
				mockActuator.EXPECT().
					GenerateMachineSets(test.clusterDeployment, test.machinePool, gomock.Any()).
					Return(test.generatedMachineSets, !test.actuatorDoNotProceed, nil)
			}
			if test.CAPI && test.generatedCAPIMachineSets != nil {
				mockActuator.EXPECT().
					GenerateCAPIMachineSets(test.clusterDeployment, test.machinePool, gomock.Any()).
					Return(test.generatedCAPIMachineSets, !test.actuatorDoNotProceed, nil)
			}

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(remoteFakeClient, nil).AnyTimes()

			logger := log.WithField("controller", "remotemachineset")
			controllerExpectations := controllerutils.NewExpectations(logger)

			rcd := &ReconcileMachinePool{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        logger,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				actuatorBuilder: func(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, masterMachine *machineapi.Machine, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error) {
					return mockActuator, nil
				},
				expectations: controllerExpectations,
			}
			_, err := rcd.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      fmt.Sprintf("%s-worker", testName),
					Namespace: testNamespace,
				},
			})

			if test.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				// Should not proceed with test validations if we expected an error from the reconcile.
				return
			}
			if err != nil && !test.expectErr {
				t.Errorf("unexpected error: %v", err)
				return
			}

			pool := getPool(fakeClient, "worker")
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
							assert.Equal(t, *eMS.Spec.Replicas, *rMS.Spec.Replicas)
							assert.Equal(t, eMS.Generation, rMS.Generation)
							if !reflect.DeepEqual(eMS.ObjectMeta.Labels, rMS.ObjectMeta.Labels) {
								t.Errorf("machineset %v has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Labels, rMS.Labels)
							}
							if !reflect.DeepEqual(eMS.ObjectMeta.Annotations, rMS.ObjectMeta.Annotations) {
								t.Errorf("machineset %v has unexpected annotations:\nexpected: %v\nactual: %v", eMS.Name, eMS.Annotations, rMS.Annotations)
							}
							if !reflect.DeepEqual(eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels) {
								t.Errorf("machineset %v machinespec has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels)
							}
							if !reflect.DeepEqual(eMS.Spec.Template.Spec.Taints, rMS.Spec.Template.Spec.Taints) {
								t.Errorf("machineset %v has unexpected taints:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Taints, rMS.Spec.Template.Spec.Taints)
							}

							rAWSProviderSpec, _ := decodeAWSMachineProviderSpec(
								rMS.Spec.Template.Spec.ProviderSpec.Value, scheme.Scheme)
							log.Debugf("remote AWS: %v", printAWSMachineProviderConfig(rAWSProviderSpec))
							assert.NotNil(t, rAWSProviderSpec)

							eAWSProviderSpec, _ := decodeAWSMachineProviderSpec(
								eMS.Spec.Template.Spec.ProviderSpec.Value, scheme.Scheme)
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
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.StringPtr("GoneNotComingBack"))
				m.Status.ErrorMessage = pointer.StringPtr("The machine is not found")
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
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.StringPtr("GoneNotComingBack"))
				m.Status.ErrorMessage = pointer.StringPtr("The machine is not found")
				return m
			}(),
			func() *machineapi.Machine {
				m := testMachineSetMachine("machine-3", "worker", testName)
				m.Status.ErrorReason = (*machineapi.MachineStatusError)(pointer.StringPtr("InsufficientResources"))
				m.Status.ErrorMessage = pointer.StringPtr("No available quota")
				return m
			}(),
			testMachineSet(testName, "worker", false, 3, 0),
		},
		reason: "MultipleMachinesFailed",
		message: `Machine machine-2 failed (GoneNotComingBack): The machine is not found,
Machine machine-3 failed (InsufficientResources): No available quota,
`,
	}}

	machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := fake.NewClientBuilder().WithRuntimeObjects(test.existing...).Build()

			ms := &machineapi.MachineSet{}
			err := fake.Get(context.TODO(), types.NamespacedName{Namespace: machineAPINamespace, Name: testName}, ms)
			require.NoError(t, err)

			reason, message := summarizeMachinesError(fake, ms, log.WithField("controller", "remotemachineset"))
			assert.Equal(t, test.reason, reason)
			assert.Equal(t, test.message, message)
		})
	}
}

func testMachinePool() *hivev1.MachinePool {
	return &hivev1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "MachinePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  testNamespace,
			Name:       fmt.Sprintf("%s-%s", testName, testPoolName),
			Finalizers: []string{finalizer},
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testName,
			},
			Name:     testPoolName,
			Replicas: pointer.Int64Ptr(3),
			Platform: hivev1.MachinePoolPlatform{
				AWS: &hivev1aws.MachinePoolPlatform{
					InstanceType: testInstanceType,
				},
			},
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": testPoolName,
				"machine.openshift.io/cluster-api-machine-type": testPoolName,
			},
			Taints: []corev1.Taint{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: hivev1.MachinePoolStatus{
			Conditions: []hivev1.MachinePoolCondition{
				{
					Status: corev1.ConditionUnknown,
					Type:   hivev1.NotEnoughReplicasMachinePoolCondition,
				},
				{
					Status: corev1.ConditionUnknown,
					Type:   hivev1.NoMachinePoolNameLeasesAvailable,
				},
				{
					Status: corev1.ConditionUnknown,
					Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				},
				{
					Status: corev1.ConditionUnknown,
					Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				},
			},
		},
	}
}

func testAutoscalingMachinePool(min, max int) *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Replicas = nil
	p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
		MinReplicas: int32(min),
		MaxReplicas: int32(max),
	}
	for i, cond := range p.Status.Conditions {
		// Condition will always be present because it is initialized in testMachinePool
		if cond.Type == hivev1.NotEnoughReplicasMachinePoolCondition {
			cond.Status = corev1.ConditionFalse
			cond.Reason = "EnoughReplicas"
			p.Status.Conditions[i] = cond
		}
	}
	return p
}

func testAWSProviderSpec() *awsprovider.AWSMachineProviderConfig {
	return &awsprovider.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AWSMachineProviderConfig",
			APIVersion: awsprovider.SchemeGroupVersion.String(),
		},
		AMI: awsprovider.AWSResourceReference{
			ID: aws.String(testAMI),
		},
	}
}

func testMachineSpec(machineType string) machineapi.MachineSpec {
	rawAWSProviderSpec, err := encodeAWSMachineProviderSpec(testAWSProviderSpec(), scheme.Scheme)
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

func testCAPIMachineSpec(machineType string) capiv1.MachineSpec {
	dataSecretName := fmt.Sprintf("%s-user-data", machineType)
	return capiv1.MachineSpec{
		Bootstrap: capiv1.Bootstrap{
			DataSecretName: &dataSecretName,
		},
		ClusterName: testName,
		InfrastructureRef: corev1.ObjectReference{
			Namespace:  testName,
			Name:       testName,
			APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
			Kind:       "AWSMachineTemplate",
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

func testMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int) *machineapi.MachineSet {
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
	return &ms
}

func testCAPIMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int) *capiv1.MachineSet {
	msReplicas := int32(replicas)
	ms := capiv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: capiv1.GroupVersion.String(),
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
		Spec: capiv1.MachineSetSpec{
			Replicas: &msReplicas,
			Template: capiv1.MachineTemplateSpec{
				ObjectMeta: capiv1.ObjectMeta{
					Labels: map[string]string{
						"machine.openshift.io/cluster-api-machineset": name,
						"machine.openshift.io/cluster-api-cluster":    testInfraID,
					},
				},
				Spec: testCAPIMachineSpec(machineType),
			},
		},
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
				constants.VersionMajorMinorPatchLabel: "4.4.0",
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

func printAWSMachineProviderConfig(cfg *awsprovider.AWSMachineProviderConfig) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}

func withClusterVersion(cd *hivev1.ClusterDeployment, version string) *hivev1.ClusterDeployment {
	if cd.Labels == nil {
		cd.Labels = make(map[string]string, 1)
	}
	cd.Labels[constants.VersionMajorMinorPatchLabel] = version
	return cd
}
