package machinepoolresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// BuildOptions holds optional MachinePool fields. All MachinePool fields are defined in this package
// (here or via platform option types like AWSOptions); Spec.Platform is filled by PlatformFiller.
type BuildOptions struct {
	// Replicas: fixed replica count. Mutually exclusive with Autoscaling.
	Replicas *int64
	// Autoscaling: min/max replicas. Mutually exclusive with Replicas.
	Autoscaling *hivev1.MachinePoolAutoscaling
	// Labels: node labels for the MachineSet.
	Labels map[string]string
	// MachineLabels: labels on the Machine template.
	MachineLabels map[string]string
	// Taints: taints applied to nodes.
	Taints []corev1.Taint
	// ObjectMeta.Labels
	ObjectLabels map[string]string
	// ObjectMeta.Annotations
	ObjectAnnotations map[string]string
}

// BuildMachinePool builds a single MachinePool. All MachinePool-specific fields are defined in this package;
// Spec.Platform is filled by filler (e.g. AWSOptions).
//
// Fields defined in this package:
//   - TypeMeta (Kind, APIVersion)
//   - ObjectMeta (Name, Namespace, Labels, Annotations from opts)
//   - Spec.ClusterDeploymentRef, Spec.Name, Spec.Replicas or Spec.Autoscaling
//   - Spec.Labels, Spec.MachineLabels, Spec.Taints (from opts)
//   - Spec.Platform = filled by filler (machinepoolresource.AWSOptions, etc.)
func BuildMachinePool(cdName, cdNamespace, poolName string, opts *BuildOptions, filler PlatformFiller) *hivev1.MachinePool {
	if opts == nil {
		opts = &BuildOptions{Replicas: ptr.To(int64(1))}
	}

	mp := &hivev1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachinePool",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        machinePoolName(cdName, poolName),
			Namespace:   cdNamespace,
			Labels:      opts.ObjectLabels,
			Annotations: opts.ObjectAnnotations,
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{Name: cdName},
			Name:                 poolName,
			Replicas:             opts.Replicas,
			Autoscaling:          opts.Autoscaling,
			Labels:               opts.Labels,
			MachineLabels:        opts.MachineLabels,
			Taints:               opts.Taints,
		},
	}
	if filler != nil {
		filler.FillPlatform(mp)
	}
	return mp
}

// BuildMachinePoolWithReplicas is a convenience wrapper: builds a MachinePool with only Replicas set.
func BuildMachinePoolWithReplicas(cdName, cdNamespace, poolName string, replicas int64, filler PlatformFiller) *hivev1.MachinePool {
	return BuildMachinePool(cdName, cdNamespace, poolName, &BuildOptions{Replicas: ptr.To(replicas)}, filler)
}

// machinePoolName returns the MachinePool resource name: cdName-poolName.
func machinePoolName(cdName, poolName string) string {
	return fmt.Sprintf("%s-%s", cdName, poolName)
}
