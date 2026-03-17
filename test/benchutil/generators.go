package benchutil

import (
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// GenerateConfigMap creates a ConfigMap with optional padding payload (0 = minimal).
func GenerateConfigMap(name string, dataSize int) *corev1.ConfigMap {
	data := map[string]string{"key": "value"}
	if dataSize > 0 {
		data["payload"] = strings.Repeat("x", dataSize)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       data,
	}
}

// GenerateSecret creates a Secret with optional padding payload (0 = minimal).
func GenerateSecret(name string, dataSize int) *corev1.Secret {
	data := map[string]string{"password": "secret"}
	if dataSize > 0 {
		data["payload"] = strings.Repeat("x", dataSize)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		StringData: data,
	}
}

// GenerateDeployment creates a single-replica Deployment.
func GenerateDeployment(name string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.14.2",
					}},
				},
			},
		},
	}
}

// GenerateServiceAccount creates a ServiceAccount.
func GenerateServiceAccount(name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
}

// GenerateSyncSet creates a SyncSet.
func GenerateSyncSet(name string) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{Name: "bench-cluster"},
			},
		},
	}
}
