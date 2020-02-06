package remotemachineset

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/remotemachineset/mock"
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
	awsprovider.SchemeBuilder.AddToScheme(scheme.Scheme)

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
		err := rc.List(context.TODO(), rMSL, client.UseListOptions(&client.ListOptions{
			Raw: &metav1.ListOptions{TypeMeta: tm},
		}))
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
		err := rc.List(context.TODO(), rMAL, client.UseListOptions(&client.ListOptions{
			Raw: &metav1.ListOptions{TypeMeta: tm},
		}))
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
		err := rc.List(context.TODO(), rCAL, client.UseListOptions(&client.ListOptions{
			Raw: &metav1.ListOptions{TypeMeta: tm},
		}))
		if err == nil {
			return rCAL, err
		}
		return nil, err
	}

	tests := []struct {
		name                             string
		clusterDeployment                *hivev1.ClusterDeployment
		machinePool                      *hivev1.MachinePool
		remoteExisting                   []runtime.Object
		generatedMachineSets             []*machineapi.MachineSet
		unreachable                      bool
		expectErr                        bool
		expectNoFinalizer                bool
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
			name:              "Skip create missing machine set when cluster is unreachable",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			unreachable:       true,
		},
		{
			name:              "Delete extra machine set",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			remoteExisting: []runtime.Object{
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectNoFinalizer: true,
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectNoFinalizer: true,
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Create cluster autoscaler",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Update cluster autoscaler when missing scale down",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				func() runtime.Object {
					a := testClusterAutoscaler()
					a.Spec.ScaleDown = nil
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Update cluster autoscaler when scale down disabled",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				func() runtime.Object {
					a := testClusterAutoscaler()
					a.Spec.ScaleDown.Enabled = false
					return a
				}(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Create machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler(),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Update machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 2, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
		{
			name:              "Delete machine autoscalers",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testAutoscalingMachinePool(3, 5),
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
				testMachineAutoscaler("foo-12345-worker-us-east-1d", 1, 1),
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
				*testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				*testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
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
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testClusterAutoscaler(),
				testMachineAutoscaler("foo-12345-worker-us-east-1a", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1b", 1, 2),
				testMachineAutoscaler("foo-12345-worker-us-east-1c", 1, 1),
			},
			expectNoFinalizer: true,
			expectedRemoteClusterAutoscalers: []autoscalingv1.ClusterAutoscaler{
				*testClusterAutoscaler(),
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)
		autoscalingv1.SchemeBuilder.AddToScheme(scheme.Scheme)
		autoscalingv1beta1.SchemeBuilder.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			localExisting := []runtime.Object{}
			if test.clusterDeployment != nil {
				localExisting = append(localExisting, test.clusterDeployment)
			}
			if test.machinePool != nil {
				localExisting = append(localExisting, test.machinePool)
			}
			fakeClient := fake.NewFakeClient(localExisting...)
			remoteFakeClient := fake.NewFakeClient(test.remoteExisting...)

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockActuator := mock.NewMockActuator(mockCtrl)
			if test.generatedMachineSets != nil {
				mockActuator.EXPECT().
					GenerateMachineSets(test.clusterDeployment, test.machinePool, gomock.Any()).
					Return(test.generatedMachineSets, nil)
			}

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Unreachable().Return(test.unreachable).AnyTimes()
			mockRemoteClientBuilder.EXPECT().Build().Return(remoteFakeClient, nil).AnyTimes()

			rcd := &ReconcileRemoteMachineSet{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "remotemachineset"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				actuatorBuilder: func(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error) {
					return mockActuator, nil
				},
			}
			_, err := rcd.Reconcile(reconcile.Request{
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

			if pool := getPool(fakeClient, "worker"); assert.NotNil(t, pool, "missing machinepool") {
				if test.expectNoFinalizer {
					assert.NotContains(t, pool.Finalizers, finalizer, "unexpected finalizer")
				} else {
					assert.Contains(t, pool.Finalizers, finalizer, "missing finalizer")
				}
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
								t.Errorf("machineset %v has unexpected annotations:\nexpected: %v\nactual: %v", eMS.Name, eMS.Labels, rMS.Labels)
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

func testMachinePool() *hivev1.MachinePool {
	return &hivev1.MachinePool{
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
	}
}

func testAutoscalingMachinePool(min, max int) *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Replicas = nil
	p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
		MinReplicas: int32(min),
		MaxReplicas: int32(max),
	}
	return p
}

func testMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int) *machineapi.MachineSet {
	msReplicas := int32(replicas)
	awsProviderSpec := &awsprovider.AWSMachineProviderConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AWSMachineProviderConfig",
			APIVersion: awsprovider.SchemeGroupVersion.String(),
		},
		AMI: awsprovider.AWSResourceReference{
			ID: aws.String(testAMI),
		},
	}
	rawAWSProviderSpec, err := encodeAWSMachineProviderSpec(awsProviderSpec, scheme.Scheme)
	if err != nil {
		log.WithError(err).Fatal("error encoding AWS machine provider spec")
	}

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
			},
			Generation: int64(generation),
		},
		Spec: machineapi.MachineSetSpec{
			Replicas: &msReplicas,
			Template: machineapi.MachineTemplateSpec{
				Spec: machineapi.MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
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
				},
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

func testMachineAutoscaler(name string, min, max int) *autoscalingv1beta1.MachineAutoscaler {
	return &autoscalingv1beta1.MachineAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "openshift-machine-api",
			Name:      name,
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

func testClusterAutoscaler() *autoscalingv1.ClusterAutoscaler {
	return &autoscalingv1.ClusterAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
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
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
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
	}
}

func printAWSMachineProviderConfig(cfg *awsprovider.AWSMachineProviderConfig) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}
