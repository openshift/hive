package benchutil

import (
	"strings"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// payload returns pairs with an optional padding entry (0 = minimal). Shared
// by the ConfigMap and Secret generators.
func payload(pairs map[string]string, dataSize int) map[string]string {
	if dataSize > 0 {
		pairs["payload"] = strings.Repeat("x", dataSize)
	}
	return pairs
}

// GenerateConfigMap creates a ConfigMap with optional padding payload (0 = minimal).
func GenerateConfigMap(name string, dataSize int) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       payload(map[string]string{"key": "value"}, dataSize),
	}
}

// GenerateSecret creates a Secret with optional padding payload (0 = minimal).
func GenerateSecret(name string, dataSize int) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		StringData: payload(map[string]string{"password": "secret"}, dataSize),
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

// GenerateMachineSet creates an OpenShift MachineSet -- a custom resource, as
// opposed to the built-in types above -- so benchmarks can also exercise the
// custom-resource code paths (CRD discovery, merge-patch fallback).
func GenerateMachineSet(name string) *machinev1beta1.MachineSet {
	replicas := int32(3)
	return &machinev1beta1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: machinev1beta1.MachineSetSpec{
			Replicas: &replicas,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"machine.openshift.io/cluster-api-machineset": name},
			},
		},
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
