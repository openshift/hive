package machinesets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			name: "infra-machine",
			object: &machinev1.Machine{ObjectMeta: metav1.ObjectMeta{
				Name: "infra-machine", Namespace: machineAPINamespace,
				Labels: map[string]string{"openshift.io/machine-type": infraMachinePoolName},
			}},
			artifact: "machines",
		},
		{
			name: "infra-machineset",
			object: &machinev1.MachineSet{ObjectMeta: metav1.ObjectMeta{
				Name: "infra-machineset", Namespace: machineAPINamespace,
				Labels: map[string]string{"hive.openshift.io/machine-pool": infraMachinePoolName},
			}},
			artifact: "machinesets",
		},
		{
			name: "infra-node",
			object: &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   "infra-node",
				Labels: map[string]string{"openshift.io/machine-type": infraMachinePoolName},
			}},
			artifact: "nodes",
		},
		{
			name: "machine-failed",
			object: &corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: "machine-failed", Namespace: machineAPINamespace},
				InvolvedObject: corev1.ObjectReference{Kind: "Machine", Name: "infra-machine"},
				Reason:         "FailedCreate",
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
		}).
		WithIndex(&corev1.Event{}, "involvedObject.name", func(object client.Object) []string {
			return []string{object.(*corev1.Event).InvolvedObject.Name}
		})
	for _, object := range objects {
		builder = builder.WithObjects(object.object)
	}

	artifactDir := t.TempDir()
	prefix := "SPOKE_machinepool_timeout_test"
	captureSpokeMachinePoolResources(t, context.Background(), builder.Build(), artifactDir, prefix, infraMachinePoolName)

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
		require.Equal(t, int64(machinePoolDiagnosticLogProbeBytes), *request.options.LimitBytes)
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

func TestPrioritizedEventsRetainAffectedPoolAndControllerEvents(t *testing.T) {
	baseTime := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	events := make([]corev1.Event, 0, machinePoolDiagnosticListLimit+2)
	for i := 0; i < machinePoolDiagnosticListLimit; i++ {
		events = append(events, corev1.Event{
			ObjectMeta:    metav1.ObjectMeta{Name: fmt.Sprintf("warning-%03d", i)},
			Type:          corev1.EventTypeWarning,
			LastTimestamp: metav1.NewTime(baseTime.Add(time.Duration(i) * time.Minute)),
			InvolvedObject: corev1.ObjectReference{
				Kind: "Machine", Name: fmt.Sprintf("unrelated-%03d", i),
			},
		})
	}
	events = append(events,
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "affected-infra-machine"},
			LastTimestamp:  metav1.NewTime(baseTime.Add(-time.Hour)),
			InvolvedObject: corev1.ObjectReference{Kind: "Machine", Name: "infra-machine"},
		},
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "controller-scheduling"},
			LastTimestamp:  metav1.NewTime(baseTime.Add(-2 * time.Hour)),
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "machine-api-controllers"},
		},
	)

	selected := prioritizedEvents(events, map[eventObjectKey]struct{}{
		{kind: "Machine", name: "infra-machine"}: {},
	}, machinePoolDiagnosticControllerEventReserve)

	require.Len(t, selected, machinePoolDiagnosticListLimit)
	names := make([]string, 0, len(selected))
	for i := range selected {
		names = append(names, selected[i].Name)
	}
	require.Contains(t, names, "affected-infra-machine")
	require.Contains(t, names, "controller-scheduling")
}

func TestNewestBoundedPodLogRetriesWithSmallerTail(t *testing.T) {
	target := machineAPILogTarget{podName: "machine-api-controllers", containerName: "machine-controller"}
	requests := make([]corev1.PodLogOptions, 0)
	readLogs := func(_ context.Context, _ string, options *corev1.PodLogOptions) ([]byte, error) {
		requests = append(requests, *options)
		if *options.TailLines > 2 {
			return []byte(strings.Repeat("old", machinePoolDiagnosticLogProbeBytes/3+1)[:machinePoolDiagnosticLogProbeBytes]), nil
		}
		return []byte("newest-controller-message\n"), nil
	}

	data, err := newestBoundedPodLog(context.Background(), target, false, readLogs)

	require.NoError(t, err)
	require.Equal(t, "newest-controller-message\n", string(data))
	require.Greater(t, len(requests), 1)
	for _, request := range requests {
		require.NotNil(t, request.LimitBytes)
		require.Equal(t, int64(machinePoolDiagnosticLogProbeBytes), *request.LimitBytes)
	}
	require.Equal(t, int64(2), *requests[len(requests)-1].TailLines)
}

func TestNewestBoundedPodLogUsesOneLineFallbackForOversizedLastLine(t *testing.T) {
	target := machineAPILogTarget{podName: "machine-api-controllers", containerName: "machine-controller"}
	requests := make([]corev1.PodLogOptions, 0)
	readLogs := func(_ context.Context, _ string, options *corev1.PodLogOptions) ([]byte, error) {
		requests = append(requests, *options)
		if *options.TailLines > 1 {
			return []byte(strings.Repeat("x", machinePoolDiagnosticLogProbeBytes)), nil
		}
		return []byte("discarded" + strings.Repeat("n", machinePoolDiagnosticLogLimitBytes)), nil
	}

	data, err := newestBoundedPodLog(context.Background(), target, true, readLogs)

	require.NoError(t, err)
	require.Len(t, data, machinePoolDiagnosticLogLimitBytes)
	require.Equal(t, strings.Repeat("n", machinePoolDiagnosticLogLimitBytes), string(data))
	lastRequest := requests[len(requests)-1]
	require.Equal(t, int64(1), *lastRequest.TailLines)
	require.Nil(t, lastRequest.LimitBytes)
	require.True(t, lastRequest.Previous)
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
