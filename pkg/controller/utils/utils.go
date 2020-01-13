package utils

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
)

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

const (
	concurrentControllerReconciles = 5
)

// GetConcurrentReconciles returns the number of goroutines each controller should
// use for parallel processing of their queue. For now this is a static value of 5.
// In future this may be read from an env var set by the operator, and driven by HiveConfig.
func GetConcurrentReconciles() int {
	return concurrentControllerReconciles
}

// MergeJsons will merge the global and local pull secret and return it
func MergeJsons(globalPullSecret string, localPullSecret string, cdLog log.FieldLogger) (string, error) {

	type dockerConfig map[string]interface{}
	type dockerConfigJSON struct {
		Auths dockerConfig `json:"auths"`
	}

	var mGlobal, mLocal dockerConfigJSON
	jGlobal := []byte(globalPullSecret)
	err := json.Unmarshal(jGlobal, &mGlobal)
	if err != nil {
		return "", err
	}

	jLocal := []byte(localPullSecret)
	err = json.Unmarshal(jLocal, &mLocal)
	if err != nil {
		return "", err
	}

	for k, v := range mLocal.Auths {
		if _, ok := mGlobal.Auths[k]; ok {
			cdLog.Infof("The auth for %s from cluster deployment pull secret is used instead of global pull secret", k)
		}
		mGlobal.Auths[k] = v
	}
	jMerged, err := json.Marshal(mGlobal)
	if err != nil {
		return "", err
	}
	return string(jMerged), nil
}

// GetChecksumOfObject returns the md5sum hash of the object passed in.
func GetChecksumOfObject(object interface{}) (string, error) {
	b, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(b)), nil
}

// GetChecksumOfObjects returns the md5sum hash of the objects passed in.
func GetChecksumOfObjects(objects ...interface{}) (string, error) {
	return GetChecksumOfObject(objects)
}

// DNSZoneName returns the predictable name for a DNSZone for the given ClusterDeployment.
func DNSZoneName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "zone")
}

// LogLevel returns the log level to use to log the specified error.
func LogLevel(err error) log.Level {
	if err == nil {
		return log.ErrorLevel
	}
	for {
		switch {
		case apierrors.IsAlreadyExists(err),
			apierrors.IsConflict(err),
			apierrors.IsNotFound(err):
			return log.InfoLevel
		}
		cause := errors.Cause(err)
		if cause == err {
			return log.ErrorLevel
		}
		err = cause
	}
}

// SetOwnerLabel creates a label on the given runtime object that corresponds to the name of the owner object.
// It is assumed that the owner type is known via context and that the owner is in the same namespace as the owned object.
func SetOwnerLabel(ownerLabel string, owner, object metav1.Object) {
	labels := object.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[ownerLabel] = owner.GetName()
	object.SetLabels(labels)
}

// GetRuntimeObjects queries kubernetes for a list of types.
func GetRuntimeObjects(client client.Client, typesToList []runtime.Object, opts ...client.ListOptionFunc) ([]runtime.Object, error) {
	objects := []runtime.Object{}

	for _, t := range typesToList {
		listObj := t.DeepCopyObject()
		if err := client.List(context.TODO(), listObj, opts...); err != nil {
			return nil, err
		}
		list, err := meta.ExtractList(listObj)
		if err != nil {
			return nil, err
		}

		objects = append(objects, list...)
	}

	return objects, nil
}
