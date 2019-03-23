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
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/images"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

func (r *ReconcileHiveConfig) deployHive(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder) error {

	asset := assets.MustAsset("config/manager/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)

	if r.hiveImage != "" {
		hiveDeployment.Spec.Template.Spec.Containers[0].Image = r.hiveImage
		// NOTE: overwriting all environment vars here, there are no others at the time of
		// writing:
		hiveImageEnvVar := corev1.EnvVar{
			Name:  images.HiveImageEnvVar,
			Value: r.hiveImage,
		}

		hiveDeployment.Spec.Template.Spec.Containers[0].Env = append(hiveDeployment.Spec.Template.Spec.Containers[0].Env, hiveImageEnvVar)
	}

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
		scheme.Scheme)

	// TODO: it would be nice to be able to log if there were changes or not
	// for all artifacts we Apply.

	err := h.ApplyRuntimeObject(hiveDeployment, s)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Info("deployment applied")

	// Deploy the desired ClusterImageSets representing installable releases of OpenShift.
	// TODO: in future this should be pipelined somehow.
	applyAssets := []string{
		"config/clusterimagesets/openshift-4.0-beta2.yaml",
		"config/clusterimagesets/openshift-4.0-latest.yaml",
	}
	for _, a := range applyAssets {
		err = util.ApplyAsset(h, a, hLog)
		if err != nil {
			return err
		}
	}

	hLog.Info("all hive components successfully reconciled")
	return nil
}
