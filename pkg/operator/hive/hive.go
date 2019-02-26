/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hive

import (
	"context"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivecdctrlr "github.com/openshift/hive/pkg/controller/clusterdeployment"

	"github.com/openshift/hive/pkg/operator/assets"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHiveConfig) deployHive(hLog log.FieldLogger, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	// Parse yaml for all Hive objects:
	asset := assets.MustAsset("config/manager/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)

	// Set owner refs on all objects in the deployment so deleting the operator CRD
	// will clean everything up:
	// NOTE: we do not cleanup the CRDs themselves so as not to destroy data.

	if err := controllerutil.SetControllerReference(instance, hiveDeployment, r.scheme); err != nil {
		hLog.WithError(err).Info("error setting owner ref")
		return err
	}

	expectedDeploymentGen := int64(0)
	currentDeployment := &appsv1.Deployment{}
	foundCurrentDeployment := false
	err := r.Get(context.Background(), types.NamespacedName{Name: hiveDeployment.Name, Namespace: hiveDeployment.Namespace}, currentDeployment)
	if err != nil && !errors.IsNotFound(err) {
		hLog.WithError(err).Error("error looking up current deployment")
		return err
	} else if err == nil {
		expectedDeploymentGen = currentDeployment.ObjectMeta.Generation
		foundCurrentDeployment = true
	}

	if instance.Spec.Image != "" {
		hiveDeployment.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image
		// NOTE: overwriting all environment vars here, there are no others at the time of
		// writing:
		hiveDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{
				Name:  hivecdctrlr.HiveImageEnvVar,
				Value: instance.Spec.Image,
			},
		}
	}

	// ApplyDeployment does not check much of the Spec for changes. Do some manual
	// checking and if we see something we care about has changed, force an update
	// by changing the expected deployment generation.
	if foundCurrentDeployment && currentDeployment.Spec.Template.Spec.Containers[0].Image !=
		hiveDeployment.Spec.Template.Spec.Containers[0].Image {
		hLog.WithFields(log.Fields{
			"current": currentDeployment.Spec.Template.Spec.Containers[0].Image,
			"new":     hiveDeployment.Spec.Template.Spec.Containers[0].Image,
		}).Info("overriding deployment image")
		expectedDeploymentGen = expectedDeploymentGen - 1
	}

	var changed bool
	_, changed, err = resourceapply.ApplyDeployment(r.kubeClient.AppsV1(),
		recorder, hiveDeployment, expectedDeploymentGen, false)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.WithField("changed", changed).Info("deployment updated")

	hLog.Info("Hive components reconciled")
	return nil

}
