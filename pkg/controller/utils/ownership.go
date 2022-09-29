package utils

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"

	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	librarygocontroller "github.com/openshift/library-go/pkg/controller"
)

// OwnershipUniqueKey contains the uniquly identifiable pattern for ensuring ownership labels are correct applied for a type.
type OwnershipUniqueKey struct {
	LabelSelector map[string]string
	TypeToList    client.ObjectList
	Controlled    bool
}

// ReconcileOwnerReferences ensures that given owner is in fact the actual owner for all types in typesToList given a specific labelSelector
func ReconcileOwnerReferences(owner hivev1.MetaRuntimeObject, ownershipKeys []*OwnershipUniqueKey, kubeclient client.Client, scheme *runtime.Scheme, logger log.FieldLogger) error {
	errlist := []error{}

	for _, ownershipKey := range ownershipKeys {
		objects, err := ListRuntimeObjects(kubeclient, []client.ObjectList{ownershipKey.TypeToList}, client.MatchingLabels(ownershipKey.LabelSelector), client.InNamespace(owner.GetNamespace()))
		if err != nil {
			errlist = append(errlist, errors.Wrap(err, "failed listing objects owned by clusterdeployment according to label"))
			continue
		}

		for _, object := range objects {
			mrObject, ok := object.(hivev1.MetaRuntimeObject)
			if !ok {
				// This should never happen since all hive objects implement both the meta and runtime object interfaces.
				logger.Warnf("Failed converting object to MetaRuntimeObject")
				continue
			}

			err := SyncOwnerReference(owner, mrObject, kubeclient, scheme, ownershipKey.Controlled, logger)
			if err != nil {
				errlist = append(errlist, err)
			}

		}
	}

	return utilerrors.NewAggregate(errlist)
}

// SyncOwnerReference ensures that the object passed in has an owner reference of the owner passed in. It then updates the object in Kube.
// If 'controlled' is set to true, the owner is set as the controller of the object.
// BlockOwnerDeletion is set to true for all owner references
func SyncOwnerReference(owner hivev1.MetaRuntimeObject, object hivev1.MetaRuntimeObject, kubeclient client.Client,
	scheme *runtime.Scheme, controlled bool, logger log.FieldLogger) error {
	objectNamespacedName := object.GetNamespace() + "/" + object.GetName()
	ownerNamespacedName := owner.GetNamespace() + "/" + owner.GetName()

	objectGVK, err := apiutil.GVKForObject(object, scheme)
	if err != nil {
		logger.WithField("objectNamespacedName", objectNamespacedName).Warn("getting GVK for object")
		return nil // Not returning error because that could stop the overall reconciliation. Just logging a warning.
	}

	ownerGVK, err := apiutil.GVKForObject(owner, scheme)
	if err != nil {
		logger.WithField("ownerNamespacedName", ownerNamespacedName).Warn("getting GVK for owner")
		return nil // Not returning error because that could stop the overall reconciliation. Just logging a warning.
	}

	objectLogger := logger.WithFields(log.Fields{
		"ownerKind":            ownerGVK.Kind,
		"ownerNamespacedName":  ownerNamespacedName,
		"objectKind":           objectGVK.Kind,
		"objectNamespacedName": objectNamespacedName,
	})

	ownerRef := metav1.NewControllerRef(owner, ownerGVK)

	if controlled {
		// Remove any other controller ref (librarygocontroller doesn't look at controller references, so it won't do this).
		for i, ref := range object.GetOwnerReferences() {
			if ref.Controller != nil && *ref.Controller {
				if !equality.Semantic.DeepEqual(&ref, ownerRef) {
					ownerRefs := object.GetOwnerReferences()
					ownerRefs[i] = ownerRefs[len(ownerRefs)-1]              // Copy last element in the slice over the top of the controller owner reference.
					object.SetOwnerReferences(ownerRefs[:len(ownerRefs)-1]) // Remove the last element (since it's now in the position pointed to i)
				}
				break // There can be only 1 controller owner ref, so we don't need to loop after we find it.
			}
		}
	} else {
		ownerRef.Controller = nil
	}

	// Add the controller reference if it doesn't already exist.
	ownerRefsChanged := librarygocontroller.EnsureOwnerRef(object, *ownerRef)

	if !ownerRefsChanged {
		objectLogger.Debug("Object has correct ownership. No changes necessary.")
		return nil
	}

	if err := kubeclient.Update(context.TODO(), object); err != nil {
		return errors.Wrapf(err, "could not update object %v %v", objectGVK.Kind, objectNamespacedName)
	}

	objectLogger.Info("Successfully set owner reference using labels")
	return nil
}

// labelKeyForOwnerType accepts an instance of an object that can "own" a Job*, and returns the key
// of the label we put on that Job when it is owned by an object of that type. The error return is
// nil unless an unknown object type is passed in.
// *We're talking about manual "ownership" rather than the k8s variety, as the objects live on
// different servers in scale mode.
func labelKeyForOwnerType(ownerInstance client.Object) (string, error) {
	switch ownerInstance.(type) {
	case *hivev1.ClusterDeployment:
		return constants.ClusterDeploymentNameLabel, nil
	case *hivev1.ClusterProvision:
		return constants.ClusterProvisionNameLabel, nil
	case *hivev1.ClusterDeprovision:
		return constants.ClusterDeprovisionNameLabel, nil
	default:
		return "", fmt.Errorf("don't know how to get job owner for owner type %T", ownerInstance)
	}

}

// GetJobOwnerNSName returns the NamespacedName of the object that "owns" the specified job*. The
// ownerInstance is used to determine the type of the object we expect that owner to be. Returns
// nil if the specified job has no owner of that type.
// *We're talking about manual "ownership" rather than the k8s variety, as the objects live on
// different servers in scale mode.
func GetJobOwnerNSName(job *batchv1.Job, ownerInstance client.Object, logger log.FieldLogger) *types.NamespacedName {
	// Parse the instance type to get just the unqualified name
	lg := logger.WithField("job", job.Name).WithField("ownerType", fmt.Sprintf("%T", ownerInstance))
	labelKey, err := labelKeyForOwnerType(ownerInstance)
	if err != nil {
		lg.Error(err)
		return nil
	}

	var labels map[string]string
	if labels = job.GetLabels(); labels == nil {
		lg.Debug("Job is not ours: no labels found")
		return nil
	}
	var ownerName string
	if ownerName = labels[labelKey]; ownerName == "" {
		logger.Debug("Job is not ours: no name label for owner type")
		return nil
	}
	return &types.NamespacedName{
		Name: ownerName,
		// In scale mode, the Job and the owner are in different clusters.
		// However, they reside in namespaces with the same name.
		Namespace: job.Namespace,
	}
}

// MapJobToOwner returns a handler.MapFunc that maps a Job to the object that "owns" it*. The owner
// type is expected to be that of the ownerInstance.
// *We're talking about manual "ownership" rather than the k8s variety, as the objects live on
// different servers in scale mode.
func MapJobToOwner(ownerInstance client.Object, logger log.FieldLogger) func(o client.Object) []reconcile.Request {
	lg := logger.WithField("ownerType", fmt.Sprintf("%T", ownerInstance))
	return func(o client.Object) []reconcile.Request {
		job := o.(*batchv1.Job)
		if job == nil {
			lg.WithField("gotType", fmt.Sprintf("%T", o)).Error("Error mapping: got unexpected type (expected Job)")
			return nil
		}
		nsName := GetJobOwnerNSName(job, ownerInstance, logger)
		if nsName == nil {
			// GetJobOwnerNSName logged
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: *nsName,
			},
		}
	}
}

// EnsureOwnedJobsDeleted idempotently deletes all Jobs "owned" by the specified owner*.
// *We're talking about manual "ownership" rather than the k8s variety, as the objects live on
// different servers in scale mode.
func EnsureOwnedJobsDeleted(c client.Client, owner client.Object, logger log.FieldLogger) error {
	labelKey, err := labelKeyForOwnerType(owner)
	if err != nil {
		return errors.Wrap(err, "Invalid input to EnsureOwnedJobDeleted -- please report this bug")
	}
	return c.DeleteAllOf(
		context.TODO(), &batchv1.Job{},
		client.InNamespace(owner.GetNamespace()),
		client.MatchingLabels{labelKey: owner.GetName()},
		client.PropagationPolicy(metav1.DeletePropagationForeground))
}
