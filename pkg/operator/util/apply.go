package util

import (
	"fmt"

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

// ApplyConfig holds configuration options for applying runtime objects
type ApplyConfig struct {
	// NamespaceOverride overrides the namespace of the object before applying
	NamespaceOverride *string
	// HiveConfig adds an owner reference for garbage collection if provided
	HiveConfig *hivev1.HiveConfig
	// Logger for logging apply operations
	Logger log.FieldLogger
	// OverrideClusterRoleBindingSubjects overrides the namespace on ClusterRoleBinding subjects
	OverrideClusterRoleBindingSubjects bool
}

// ApplyResource applies a resource with the specified configuration options.
// This is the consolidated function that handles all input types and apply variations.
//
// Supported input types:
// - runtime.Object: applied directly
// - string: treated as asset path, loaded and decoded
// - []byte: decoded as runtime object
func ApplyResource(h resource.Helper, input interface{}, config ApplyConfig) (resource.ApplyResult, error) {
	var runtimeObj runtime.Object
	var err error

	// Convert input to runtime.Object based on type
	switch v := input.(type) {
	case runtime.Object:
		runtimeObj = v
	case string:
		// Treat as asset path
		runtimeObj, err = readRuntimeObject(v)
		if err != nil {
			return resource.UnknownApplyResult, errors.Wrapf(err, "failed to read runtime object from asset path: %s", v)
		}
	case []byte:
		// Treat as asset bytes
		runtimeObj, err = decodeRuntimeObject(v)
		if err != nil {
			return resource.UnknownApplyResult, errors.Wrap(err, "failed to decode runtime object from bytes")
		}
	default:
		return resource.UnknownApplyResult, fmt.Errorf("unsupported input type: %T (supported: runtime.Object, string, []byte)", input)
	}

	return applyRuntimeObject(h, runtimeObj, config)
}

// applyRuntimeObject applies a runtime object with the specified configuration options.
// This function is used internally by ApplyResource.
func applyRuntimeObject(h resource.Helper, runtimeObj runtime.Object, config ApplyConfig) (resource.ApplyResult, error) {
	// Apply namespace override
	if config.NamespaceOverride != nil {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return resource.UnknownApplyResult, err
		}
		obj.SetNamespace(*config.NamespaceOverride)
	}

	// Handle special ClusterRoleBinding subject namespace override
	if config.OverrideClusterRoleBindingSubjects && config.NamespaceOverride != nil {
		if rb, ok := runtimeObj.(*rbacv1.ClusterRoleBinding); ok {
			for i := range rb.Subjects {
				if rb.Subjects[i].Kind == "ServiceAccount" || rb.Subjects[i].Namespace != "" {
					rb.Subjects[i].Namespace = *config.NamespaceOverride
				}
			}
		}
	}

	// Apply owner reference for garbage collection
	if config.HiveConfig != nil {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return resource.UnknownApplyResult, err
		}
		ownerRef := v1.OwnerReference{
			APIVersion:         config.HiveConfig.APIVersion,
			Kind:               config.HiveConfig.Kind,
			Name:               config.HiveConfig.Name,
			UID:                config.HiveConfig.UID,
			BlockOwnerDeletion: ptr.To(true),
		}
		// This assumes we have full control of owner references for these resources the operator creates.
		obj.SetOwnerReferences([]v1.OwnerReference{ownerRef})
	}

	// Log the operation
	if config.Logger != nil {
		action := "applying runtime object"
		if config.HiveConfig != nil {
			action += " with GC"
		}
		config.Logger.Info(action)
	}

	// Apply the object
	return h.ApplyRuntimeObject(runtimeObj, scheme.GetScheme())
}

func DeleteAssetByPathWithNSOverride(h resource.Helper, assetPath, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
	requiredObj, err := readRuntimeObject(assetPath)
	if err != nil {
		return errors.Wrapf(err, "unable to decode asset: %s", assetPath)
	}
	return deleteRuntimeObjectWithNSOverride(h, requiredObj, namespaceOverride, hiveconfig)
}

func DeleteAssetBytesWithNSOverride(h resource.Helper, assetBytes []byte, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
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
