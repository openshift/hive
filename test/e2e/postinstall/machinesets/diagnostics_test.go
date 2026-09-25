package machinesets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	machinev1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/hive/pkg/util/scheme"
)

func TestCaptureSpokeMachinePoolResources(t *testing.T) {
	objects := []struct {
		name     string
		object   client.Object
		artifact string
	}{
		{
			name:     "infra-machine",
			object:   &machinev1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "infra-machine", Namespace: machineAPINamespace}},
			artifact: "machines",
		},
		{
			name:     "infra-machineset",
			object:   &machinev1.MachineSet{ObjectMeta: metav1.ObjectMeta{Name: "infra-machineset", Namespace: machineAPINamespace}},
			artifact: "machinesets",
		},
		{
			name:     "infra-node",
			object:   &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "infra-node"}},
			artifact: "nodes",
		},
		{
			name: "machine-failed",
			object: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "machine-failed", Namespace: machineAPINamespace},
				Reason:     "FailedCreate",
			},
			artifact: "machine-api-events",
		},
		{
			name: "node-not-ready",
			object: &corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: "node-not-ready", Namespace: "default"},
				InvolvedObject: corev1.ObjectReference{Kind: "Node", Name: "infra-node"},
				Reason:         "NodeNotReady",
			},
			artifact: "node-events",
		},
		{
			name:     "machine-api-controller",
			object:   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine-api-controller", Namespace: machineAPINamespace}},
			artifact: "machine-api-pods",
		},
	}

	builder := fake.NewClientBuilder().
		WithScheme(scheme.GetScheme()).
		WithIndex(&corev1.Event{}, "involvedObject.kind", func(object client.Object) []string {
			return []string{object.(*corev1.Event).InvolvedObject.Kind}
		})
	for _, object := range objects {
		builder = builder.WithObjects(object.object)
	}

	artifactDir := t.TempDir()
	prefix := "SPOKE_machinepool_timeout_test"
	captureSpokeMachinePoolResources(t, context.Background(), builder.Build(), artifactDir, prefix)

	for _, object := range objects {
		path := filepath.Join(artifactDir, fmt.Sprintf("%s_%s.yaml", prefix, object.artifact))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(data), object.name)
	}
}

func TestCaptureMachineAPIPodLogsContinuesAfterCurrentLogError(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "machine-api-controllers"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "machine-controller"},
				{Name: "machineset-controller"},
			}},
			Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
				{Name: "machine-controller", RestartCount: 1},
			}},
		},
	}

	type logRequest struct {
		podName string
		options corev1.PodLogOptions
	}
	requests := make([]logRequest, 0)
	readLogs := func(_ context.Context, podName string, options *corev1.PodLogOptions) ([]byte, error) {
		requests = append(requests, logRequest{podName: podName, options: *options})
		if options.Container == "machine-controller" && !options.Previous {
			return nil, fmt.Errorf("current logs unavailable")
		}
		if options.Previous {
			return []byte("previous provider logs"), nil
		}
		return []byte("current sibling logs"), nil
	}

	artifactDir := t.TempDir()
	prefix := "SPOKE_machinepool_timeout_test"
	captureMachineAPIPodLogs(t, context.Background(), pods, artifactDir, prefix, readLogs)

	require.Len(t, requests, 3)
	for _, request := range requests {
		require.Equal(t, "machine-api-controllers", request.podName)
		require.Equal(t, int64(machinePoolDiagnosticLogTailLines), *request.options.TailLines)
		require.Equal(t, int64(machinePoolDiagnosticLogLimitBytes), *request.options.LimitBytes)
	}
	require.False(t, requests[0].options.Previous)
	require.True(t, requests[1].options.Previous)

	currentPath := filepath.Join(artifactDir, prefix+"_machine-api-controllers_machine-controller.log")
	_, err := os.Stat(currentPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	previous, err := os.ReadFile(filepath.Join(artifactDir, prefix+"_machine-api-controllers_machine-controller_previous.log"))
	require.NoError(t, err)
	require.Equal(t, "previous provider logs", string(previous))
	sibling, err := os.ReadFile(filepath.Join(artifactDir, prefix+"_machine-api-controllers_machineset-controller.log"))
	require.NoError(t, err)
	require.Equal(t, "current sibling logs", string(sibling))
}

func TestMachineAPILogTargetsAreBoundedAndPrioritizeProviderController(t *testing.T) {
	pods := make([]corev1.Pod, machinePoolDiagnosticLogTargetLimit+1)
	for i := range pods {
		pods[i] = corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("controller-%02d", i)},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "other-controller"}}},
		}
	}
	pods = append(pods, corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "zz-provider-controller"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "machine-controller"}}},
	})

	targets := machineAPILogTargets(pods)

	require.Len(t, targets, machinePoolDiagnosticLogTargetLimit)
	require.Equal(t, "machine-controller", targets[0].containerName)
	require.Equal(t, "zz-provider-controller", targets[0].podName)
}

func TestBoundedLogTail(t *testing.T) {
	data := []byte("discarded" + strings.Repeat("x", machinePoolDiagnosticLogLimitBytes))

	bounded := boundedLogTail(data)

	require.Len(t, bounded, machinePoolDiagnosticLogLimitBytes)
	require.Equal(t, strings.Repeat("x", machinePoolDiagnosticLogLimitBytes), string(bounded))
}
