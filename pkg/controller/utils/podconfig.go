package utils

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SharedPodConfig is initially created from the hive-operator pod and must be included in any Jobs or Pods
// we create from downstream.
type SharedPodConfig struct {
	NodeSelector     map[string]string
	Tolerations      []corev1.Toleration
	ImagePullSecrets []corev1.LocalObjectReference
}

func ReadSharedConfigFromThisPod(cl client.Client) (*SharedPodConfig, error) {
	thisPod, err := getThisPod(cl)
	if err != nil {
		return nil, err
	}

	return &SharedPodConfig{
		NodeSelector:     thisPod.Spec.NodeSelector,
		Tolerations:      thisPod.Spec.Tolerations,
		ImagePullSecrets: thisPod.Spec.ImagePullSecrets,
	}, nil
}

// getThisPod gets this pod.  If this function is useful somewhere else, make it public again
func getThisPod(cl client.Client) (*corev1.Pod, error) {
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName == "" || podNamespace == "" {
		return nil, fmt.Errorf("missing POD_NAME (%q) and/or POD_NAMESPACE (%q) env", podName, podNamespace)
	}
	po := &corev1.Pod{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: podNamespace, Name: podName}, po); err != nil {
		return nil, err
	}
	return po, nil
}
