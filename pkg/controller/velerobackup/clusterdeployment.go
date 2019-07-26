package velerobackup

import (
	"context"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *ReconcileBackup) registerClusterDeploymentWatch(c controller.Controller) error {
	// Watch for changes to ClusterDeployment
	return c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(r.clusterDeploymentWatchHandler),
	})
}

func (r *ReconcileBackup) clusterDeploymentWatchHandler(a handler.MapObject) []reconcile.Request {
	cd, ok := a.Object.(*hivev1.ClusterDeployment)
	if !ok {
		// Wasn't a ClusterDeployment, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a.Object)
		return []reconcile.Request{}
	}

	// Queue up the NS for this ClusterDeployment
	retval := []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Namespace: cd.Namespace,
		}}}
	return retval
}

func (r *ReconcileBackup) getModifiedClusterDeploymentsInNamespace(namespace string) ([]*hivev1.ClusterDeployment, error) {
	nsLogger := addNamespaceLoggerFields(r.logger, namespace)

	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(context.TODO(), allClusterDeployments, client.InNamespace(namespace))
	if err != nil {
		nsLogger.WithError(err).Error("Couldn't get list of ClusterDeployments")
		return nil, err
	}

	changedClusterDeployments := []*hivev1.ClusterDeployment{}
	for i, clusterDeployment := range allClusterDeployments.Items {
		hasChanged, err := r.hasClusterDeploymentChanged(&clusterDeployment)
		if err != nil {
			return nil, err
		}

		if hasChanged {
			// This one is different!
			changedClusterDeployments = append(changedClusterDeployments, &allClusterDeployments.Items[i])
		}
	}

	return changedClusterDeployments, nil
}

func (r *ReconcileBackup) hasClusterDeploymentChanged(clusterDeployment *hivev1.ClusterDeployment) (bool, error) {
	cdLogger := addClusterDeploymentLoggerFields(r.logger, clusterDeployment)

	newChecksum, err := r.clusterDeploymentChecksum(clusterDeployment)
	if err != nil {
		cdLogger.WithError(err).Info("error calculating checksum of cluster deployment")
		return false, err
	}

	// Make sure newChecksum is different than the old checksum.
	oldChecksum := clusterDeployment.Annotations[controllerutils.LastBackupAnnotation]
	return newChecksum != oldChecksum, nil
}

func (r *ReconcileBackup) clusterDeploymentChecksum(clusterDeployment *hivev1.ClusterDeployment) (string, error) {
	// We need to take the checksum WITHOUT the previous checksum
	// to avoid an infinite loop of always changing the checksum.
	metaCopy := clusterDeployment.ObjectMeta.DeepCopy()
	delete(metaCopy.Annotations, controllerutils.LastBackupAnnotation)

	// We also need to take the checksum WITHOUT the previous ResourceVersion
	// as when we update the checksum annotation, ResourceVersion changes.
	metaCopy.ResourceVersion = ""

	return r.checksumFunc(metaCopy, clusterDeployment.Spec)
}

func (r *ReconcileBackup) updateClusterDeploymentLastBackupChecksum(clusterDeployments []*hivev1.ClusterDeployment) error {
	errors := []error{}

	for _, clusterDeployment := range clusterDeployments {
		checksumStr, err := r.clusterDeploymentChecksum(clusterDeployment)

		if err != nil {
			aggregate, ok := err.(utilerrors.Aggregate)
			if ok {
				// Unpack the aggregate error so that we don't return an aggregate of an aggregate.
				errors = append(errors, aggregate.Errors()...)
			} else {
				errors = append(errors, err)
			}

			continue // It errored, so jump to next loop.
		}

		// Make sure Annotations is a map.
		if clusterDeployment.Annotations == nil {
			clusterDeployment.Annotations = map[string]string{}
		}

		// Actually set the checksum
		clusterDeployment.Annotations[controllerutils.LastBackupAnnotation] = checksumStr

		err = r.Update(context.TODO(), clusterDeployment)
		if err != nil {
			errors = append(errors, err)
			continue // It errored, so jump to next loop.
		}
	}

	return utilerrors.NewAggregate(errors)
}

func addClusterDeploymentLoggerFields(logger log.FieldLogger, cd *hivev1.ClusterDeployment) *log.Entry {
	return logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
}

func addNamespaceLoggerFields(logger log.FieldLogger, namespace string) *log.Entry {
	return logger.WithFields(log.Fields{
		"namespace": namespace,
	})
}
