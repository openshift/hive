package utils

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"

	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	librarygocontroller "github.com/openshift/library-go/pkg/controller"
)

// OwnershipUniqueKey contains the uniquly identifiable pattern for ensuring ownership labels are correct applied for a type.
type OwnershipUniqueKey struct {
	LabelSelector map[string]string
	TypeToList    runtime.Object
	Controlled    bool
}

// ReconcileOwnerReferences ensures that given owner is in fact the actual owner for all types in typesToList given a specific labelSelector
func ReconcileOwnerReferences(owner hivev1.MetaRuntimeObject, ownershipKeys []*OwnershipUniqueKey, kubeclient client.Client, scheme *runtime.Scheme, logger log.FieldLogger) error {
	errlist := []error{}

	for _, ownershipKey := range ownershipKeys {
		objects, err := ListRuntimeObjects(kubeclient, []runtime.Object{ownershipKey.TypeToList}, client.MatchingLabels(ownershipKey.LabelSelector), client.InNamespace(owner.GetNamespace()))
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
