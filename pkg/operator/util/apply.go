package util

import (
	"strings"

	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/scheme"
)

// ApplyAsset loads a path from our bindata assets and applies it to the cluster. This function does not apply
// a HiveConfig owner reference for garbage collection, and should only be used for resources we explicitly want
// to leave orphaned when Hive is uninstalled. See ApplyAssetWithGC for the more common use case.
func ApplyAsset(h resource.Helper, assetPath string, hLog log.FieldLogger) error {
	assetLog := hLog.WithField("asset", assetPath)
	assetLog.Debug("reading asset")
	asset := assets.MustAsset(assetPath)
	assetLog.Debug("applying asset")
	result, err := h.Apply(asset)
	if err != nil {
		assetLog.WithError(err).Error("error applying asset")
		return err
	}
	assetLog.Infof("asset applied successfully: %v", result)
	return nil
}

// ApplyAssetWithGC loads a path from our bindata assets, adds an OwnerReference to the HiveConfig
// for garbage collection (used when uninstalling Hive), and applies it to the cluster.
func ApplyAssetWithGC(h resource.Helper, assetPath string, hc *hivev1.HiveConfig, hLog log.FieldLogger) error {
	assetLog := hLog.WithField("asset", assetPath)
	assetLog.Info("reading asset")
	runtimeObj, err := readRuntimeObject(assetPath)
	if err != nil {
		return err
	}
	assetLog.Info("applying asset with GC")
	result, err := ApplyRuntimeObjectWithGC(h, runtimeObj, hc)
	if err != nil {
		assetLog.WithError(err).Error("error applying asset")
		return err
	}
	assetLog.Infof("asset applied successfully: %v", result)
	return nil
}

// ApplyAssetWithNSOverrideAndGC loads the given asset, overrides the namespace, adds an owner reference to
// HiveConfig for uninstall, and applies it to the cluster.
func ApplyAssetWithNSOverrideAndGC(h resource.Helper, assetPath, namespaceOverride string, hiveConfig *hivev1.HiveConfig) error {
	requiredObj, err := readRuntimeObject(assetPath)
	if err != nil {
		return errors.Wrapf(err, "unable to decode asset: %s", assetPath)
	}
	obj, _ := meta.Accessor(requiredObj)
	obj.SetNamespace(namespaceOverride)
	_, err = ApplyRuntimeObjectWithGC(h, requiredObj, hiveConfig)
	if err != nil {
		return errors.Wrapf(err, "unable to apply asset: %s", assetPath)
	}
	return nil
}

func DeleteAssetWithNSOverride(h resource.Helper, assetPath, namespaceOverride string, hiveconfig *hivev1.HiveConfig) error {
	requiredObj, err := readRuntimeObject(assetPath)
	if err != nil {
		return errors.Wrapf(err, "unable to decode asset: %s", assetPath)
	}
	objA, _ := meta.Accessor(requiredObj)
	objT, _ := meta.TypeAccessor(requiredObj)
	if err := h.Delete(objT.GetAPIVersion(), objT.GetKind(), namespaceOverride, objA.GetName()); err != nil {
		return errors.Wrapf(err, "unable to delete asset: %s", assetPath)
	}
	return nil
}

// ApplyClusterRoleBindingAssetWithSubjectNSOverrideAndGC loads the given asset, overrides the namespace on the subject,
// adds an owner reference to HiveConfig for uninstall, and applies it to the cluster.
func ApplyClusterRoleBindingAssetWithSubjectNSOverrideAndGC(h resource.Helper, roleBindingAssetPath, namespaceOverride string, hiveConfig *hivev1.HiveConfig) error {

	rb := resourceread.ReadClusterRoleBindingV1OrDie(assets.MustAsset(roleBindingAssetPath))
	for i := range rb.Subjects {
		if rb.Subjects[i].Kind == "ServiceAccount" || rb.Subjects[i].Namespace != "" {
			rb.Subjects[i].Namespace = namespaceOverride
		}
	}
	_, err := ApplyRuntimeObjectWithGC(h, rb, hiveConfig)
	if err != nil {
		return errors.Wrapf(err, "unable to apply asset: %s", roleBindingAssetPath)
	}
	return nil
}

// ApplyRuntimeObjectWithGC adds an OwnerReference to the HiveConfig on the runtime object, and applies it to the cluster.
func ApplyRuntimeObjectWithGC(h resource.Helper, runtimeObj runtime.Object, hc *hivev1.HiveConfig) (resource.ApplyResult, error) {
	obj, err := meta.Accessor(runtimeObj)
	if err != nil {
		return resource.UnknownApplyResult, err
	}
	ownerRef := v1.OwnerReference{
		APIVersion:         hc.APIVersion,
		Kind:               hc.Kind,
		Name:               hc.Name,
		UID:                hc.UID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	// This assumes we have full control of owner references for these resources the operator creates.
	obj.SetOwnerReferences([]v1.OwnerReference{ownerRef})
	return h.ApplyRuntimeObject(runtimeObj, scheme.GetScheme())
}

func readRuntimeObject(assetPath string) (runtime.Object, error) {
	obj, _, err := serializer.NewCodecFactory(scheme.GetScheme()).UniversalDeserializer().
		Decode(assets.MustAsset(assetPath), nil, nil)
	return obj, err
}

// IsNoSuchCRD inspects an error and determines whether it is similar to this:
// "could not get mapping: no matches for kind \"ServiceMonitor\" in version \"monitoring.coreos.com/v1\""
// In certain circumstances -- e.g. deploying hive on non-openshift with monitoring disabled -- these
// errors may be spurious and ignorable.
func IsNoSuchCRD(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no matches for kind")
}
