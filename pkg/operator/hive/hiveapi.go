package hive

import (
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/resource"
)

func (r *ReconcileHiveConfig) tearDownLegacyHiveAPI(hLog log.FieldLogger, hiveNSName string) error {
	objects := []struct {
		key    client.ObjectKey
		object runtime.Object
	}{
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi"},
			object: &appsv1.Deployment{},
		},
		{
			key:    client.ObjectKey{Name: "v1alpha1.hive.openshift.io"},
			object: &apiregistrationv1.APIService{},
		},
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi"},
			object: &corev1.Service{},
		},
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi-sa"},
			object: &corev1.ServiceAccount{},
		},
	}

	errorList := []error{}
	for _, obj := range objects {
		if err := resource.DeleteAnyExistingObject(r, obj.key, obj.object, hLog); err != nil {
			errorList = append(errorList, err)
			hLog.WithError(err).Warn("failed to clean up old aggregated API server object")
		}
	}

	return errors.NewAggregate(errorList)
}
