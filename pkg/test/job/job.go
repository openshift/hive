package job

import (
	"github.com/openshift/hive/pkg/test/generic"
	batchv1 "k8s.io/api/batch/v1"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*batchv1.Job)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *batchv1.Job {
	retval := &batchv1.Job{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(checkpoint *batchv1.Job) {
		opt(checkpoint)
	}
}
