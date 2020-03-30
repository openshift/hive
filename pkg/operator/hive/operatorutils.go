package hive

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	coreDeserializer = serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
)

func getHiveNamespace(config *hivev1.HiveConfig) string {
	if config.Spec.TargetNamespace == "" {
		return constants.DefaultHiveNamespace
	}

	return config.Spec.TargetNamespace
}

func applyAssetWithNamespaceOverride(h *resource.Helper, assetPath, namespaceOverride string) error {
	requiredObj, _, err := coreDeserializer.Decode(assets.MustAsset(assetPath), nil, nil)
	if err != nil {
		return errors.Wrapf(err, "unable to decode asset: %s", assetPath)
	}
	obj, _ := meta.Accessor(requiredObj)
	obj.SetNamespace(namespaceOverride)
	_, err = h.ApplyRuntimeObject(requiredObj, scheme.Scheme)
	if err != nil {
		return errors.Wrapf(err, "unable to apply asset: %s", assetPath)
	}
	return nil
}

func applyClusterRoleBindingAssetWithSubjectNamespaceOverride(h *resource.Helper, roleBindingAssetPath, namespaceOverride string) error {
	rb := resourceread.ReadClusterRoleBindingV1OrDie(assets.MustAsset(roleBindingAssetPath))
	for i := range rb.Subjects {
		if rb.Subjects[i].Kind == "ServiceAccount" || rb.Subjects[i].Namespace != "" {
			rb.Subjects[i].Namespace = namespaceOverride
		}
	}
	_, err := h.ApplyRuntimeObject(rb, scheme.Scheme)
	if err != nil {
		return errors.Wrapf(err, "unable to apply asset: %s", roleBindingAssetPath)
	}
	return nil
}

func dynamicDelete(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, name string, hLog log.FieldLogger) error {
	rLog := hLog.WithFields(log.Fields{
		"gvr":  gvr,
		"name": name,
	})
	err := dynamicClient.Resource(gvr).Delete(name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		rLog.WithError(err).Error("error deleting resource")
		return err
	}
	return nil
}
