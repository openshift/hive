package hive

import (
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/scheme"
)

// rtoApplyOpt (runtime.Object apply option) modifies a runtime.Object in preparation for applying it.
type rtoApplyOpt func(runtime.Object, log.FieldLogger) error

// withGarbageCollection returns a RTOApplyOpt that adds an owner reference to parent to the
// runtime object so the latter gets cleaned up when the parent is deleted. Errors only happen
// if the runtime object can't be interpreted as a metav1.Object or meta.Type.
func withGarbageCollection(parent v1.Object) rtoApplyOpt {
	return func(runtimeObj runtime.Object, hLog log.FieldLogger) error {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return errors.Wrap(err, "could not interpret runtime object as metav1.Object")
		}
		parentT, err := meta.TypeAccessor(parent)
		if err != nil {
			return errors.Wrap(err, "could not interpret runtime object as meta.Type")
		}
		kind, name := parentT.GetKind(), parent.GetName()
		hLog.WithFields(log.Fields{
			"ownerKind": kind,
			"ownerName": name,
		}).Info("adding owner reference for garbage collection")
		ownerRef := v1.OwnerReference{
			APIVersion:         parentT.GetAPIVersion(),
			Kind:               kind,
			Name:               name,
			UID:                parent.GetUID(),
			BlockOwnerDeletion: ptr.To(true),
		}
		// This assumes we have full control of owner references for these resources the operator creates.
		obj.SetOwnerReferences([]v1.OwnerReference{ownerRef})
		return nil
	}
}

// withNamespaceOverride returns a RTOApplyOpt that sets the namespace of the runtime object.
// There are no error cases.
func withNamespaceOverride(namespaceOverride string) rtoApplyOpt {
	return func(runtimeObj runtime.Object, hLog log.FieldLogger) error {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return errors.Wrap(err, "could not interpret runtime object as metav1.Object")
		}
		hLog.WithField("newNamespace", namespaceOverride).Info("overriding namespace")
		obj.SetNamespace(namespaceOverride)
		return nil
	}
}

// crbWithSubjectNSOverride sets the namespace of each Subject to namespaceOverride if it is
// - a ServiceAccount subject
// - otherwise unset
// Errors if the runtime object is not a *ClusterRoleBinding
func crbWithSubjectNSOverride(namespaceOverride string) rtoApplyOpt {
	return func(rto runtime.Object, hLog log.FieldLogger) error {
		rb, ok := rto.(*rbacv1.ClusterRoleBinding)
		if !ok {
			return errors.New("object is not a ClusterRoleBinding")
		}
		for i := range rb.Subjects {
			if rb.Subjects[i].Kind == "ServiceAccount" || rb.Subjects[i].Namespace != "" {
				rb.Subjects[i].Namespace = namespaceOverride
			}
		}
		return nil
	}
}

// toRuntimeObject defines a function that produces a runtime object. It is intended for use
// in closures to supply such objects from different sources (asset paths, byte arrays) to
// ApplyRuntimeObject().
type toRuntimeObject func(log.FieldLogger) (runtime.Object, error)

// passthrough's func just returns the input runtime object.
func passthrough(rto runtime.Object) toRuntimeObject {
	return func(fl log.FieldLogger) (runtime.Object, error) {
		return rto, nil
	}
}

// fromAssetPath's func loads a runtime object from a known asset path in bindata.
func fromAssetPath(assetPath string) toRuntimeObject {
	return func(hLog log.FieldLogger) (runtime.Object, error) {
		hLog.WithField("assetPath", assetPath).Info("loading runtime object from asset")
		return readRuntimeObject(assetPath)
	}
}

// crbFromAssetPath is a special case of FromAssetPath that returns a *ClusterRoleBinding
// (a specific instance of a runtime object) from a known asset path in bindata. Panics if
// the asset is not a CRB, or if the asset can't be loaded from the specified path.
func crbFromAssetPath(roleBindingAssetPath string) toRuntimeObject {
	return func(hLog log.FieldLogger) (runtime.Object, error) {
		hLog.WithField("assetPath", roleBindingAssetPath).Info("loading ClusterRoleBinding from asset")
		return resourceread.ReadClusterRoleBindingV1OrDie(assets.MustAsset(roleBindingAssetPath)), nil
	}
}

// fromBytes produces a func that decodes a byte array into a runtime object.
func fromBytes(assetBytes []byte) toRuntimeObject {
	return func(hLog log.FieldLogger) (runtime.Object, error) {
		hLog.Info("decoding runtime object from bytes")
		return decodeRuntimeObject(assetBytes)
	}
}

// applyRuntimeObject
// - Executes rtoFactory to produce a runtime object.
// - Modifies the runtime object according to opts.
// - Applies the runtime object to the cluster via h.
func applyRuntimeObject(h resource.Helper, rtoFactory toRuntimeObject, hLog log.FieldLogger, opts ...rtoApplyOpt) (resource.ApplyResult, error) {
	requiredObj, err := rtoFactory(hLog)
	if err != nil {
		hLog.WithError(err).Error("failed to convert to runtime object")
		return resource.UnknownApplyResult, err
	}
	for _, opt := range opts {
		if err := opt(requiredObj, hLog); err != nil {
			hLog.WithError(err).Error("failed to apply option to runtime object")
			return resource.UnknownApplyResult, err
		}
	}
	return h.ApplyRuntimeObject(requiredObj, scheme.GetScheme())
}

func deleteAssetByPathWithNSOverride(h resource.Helper, assetPath, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
	requiredObj, err := readRuntimeObject(assetPath)
	if err != nil {
		return errors.Wrapf(err, "unable to decode asset: %s", assetPath)
	}
	return deleteRuntimeObjectWithNSOverride(h, requiredObj, namespaceOverride, hiveconfig)
}

func deleteAssetBytesWithNSOverride(h resource.Helper, assetBytes []byte, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
	rtObj, err := decodeRuntimeObject(assetBytes)
	if err != nil {
		return errors.Wrap(err, "unable to decode asset")
	}
	return deleteRuntimeObjectWithNSOverride(h, rtObj, namespaceOverride, hiveconfig)
}

func deleteRuntimeObjectWithNSOverride(h resource.Helper, requiredObj runtime.Object, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
	objA, _ := meta.Accessor(requiredObj)
	objT, _ := meta.TypeAccessor(requiredObj)
	if err := h.Delete(objT.GetAPIVersion(), objT.GetKind(), namespaceOverride, objA.GetName()); err != nil {
		return errors.Wrapf(err, "unable to delete asset")
	}
	return nil
}

func readRuntimeObject(assetPath string) (runtime.Object, error) {
	return decodeRuntimeObject(assets.MustAsset(assetPath))
}

func decodeRuntimeObject(assetBytes []byte) (runtime.Object, error) {
	obj, _, err := serializer.NewCodecFactory(scheme.GetScheme()).UniversalDeserializer().Decode(assetBytes, nil, nil)
	return obj, err
}
