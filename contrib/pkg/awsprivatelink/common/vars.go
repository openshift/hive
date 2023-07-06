package common

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	CredsSecret   *corev1.Secret
	DynamicClient client.Client
)
