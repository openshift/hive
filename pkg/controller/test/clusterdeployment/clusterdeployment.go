package clusterdeployment

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// Option defines a function signature for any function that wants to be passed into BuildClusterDeployment
type Option func(*hivev1.ClusterDeployment)

// Checksum defines a function signature for checksumming objects.
type Checksum func(*hivev1.ClusterDeployment) (string, error)

// BuildClusterDeployment runs each of the functions passed in to generate a cluster deployment.
func BuildClusterDeployment(opts ...Option) *hivev1.ClusterDeployment {
	retval := &hivev1.ClusterDeployment{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

// WithName sets the ClusterDeployment.Name field when building a clusterdeployment with BuildClusterDeployment.
func WithName(name string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Name = name
	}
}

// WithNamePostfix appends the string passed in to the ClusterDeployment.Name field when building a clusterdeployment with BuildClusterDeployment.
func WithNamePostfix(postfix string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Name = clusterDeployment.Name + "-" + postfix
	}
}

// WithNamespace sets the ClusterDeployment.Namespace field when building a clusterdeployment with BuildClusterDeployment.
func WithNamespace(namespace string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Namespace = namespace
	}
}

// WithAnnotationsSet ensures that ClusterDeployment.Annotations is not nil.
func WithAnnotationsSet() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		// Only set if Nil (don't wipe out existing)
		if clusterDeployment.Annotations == nil {
			clusterDeployment.Annotations = map[string]string{}
		}
	}
}

// WithEmptyAnnotations ensures that annotations is set and that map is empty.
func WithEmptyAnnotations() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		// Wipe out existing so that it's actually empty.
		clusterDeployment.Annotations = map[string]string{}
	}
}

// WithEmptyBackupChecksum ensures that the checksum annotation is empty.
func WithEmptyBackupChecksum() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		// Case where annotations don't exist, so checksum doesn't exist.
		if clusterDeployment.ObjectMeta.Annotations == nil {
			return
		}

		// Case where Annotations exist, -and- checksum annotation exists.
		delete(clusterDeployment.Annotations, controllerutils.LastBackupAnnotation)

		// If that was the only entry, then we should nil out annotations (this makes comparisons with FakeClient work).
		if len(clusterDeployment.Annotations) == 0 {
			clusterDeployment.Annotations = nil
		}
	}
}

// WithBackupChecksum sets the object's checksum attribute to the correct value.
func WithBackupChecksum(checksum Checksum) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		// In order for the checksum to be correct, the checksum needs to be calculated AFTER all other changes have been made
		checksum, err := checksum(clusterDeployment)
		if err != nil {
			// On error, panic as this should never happen.
			panic(err)
		}

		WithAnnotationsSet()(clusterDeployment)
		clusterDeployment.Annotations[controllerutils.LastBackupAnnotation] = checksum
	}
}
