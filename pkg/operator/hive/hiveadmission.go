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
	"bytes"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

func (r *ReconcileHiveConfig) deployHiveAdmission(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	asset := assets.MustAsset("config/hiveadmission/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveAdmDeployment := resourceread.ReadDeploymentV1OrDie(asset)

	err := util.ApplyAsset(h, "config/hiveadmission/service.yaml", hLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/hiveadmission/service-account.yaml", hLog)
	if err != nil {
		return err
	}

	if r.hiveImage != "" {
		hiveAdmDeployment.Spec.Template.Spec.Containers[0].Image = r.hiveImage
	}

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
		scheme.Scheme)

	buf := bytes.NewBuffer([]byte{})
	err = s.Encode(hiveAdmDeployment, buf)
	if err != nil {
		hLog.WithError(err).Error("error encoding deployment")
		return err
	}
	err = h.Apply(buf.Bytes())
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Info("deployment applied")

	applyAssets := []string{
		"config/hiveadmission/apiservice.yaml",
		"config/hiveadmission/clusterdeployment-webhook.yaml",
		"config/hiveadmission/clusterimageset-webhook.yaml",
		"config/hiveadmission/dnszones-webhook.yaml",
	}
	for _, a := range applyAssets {
		err = util.ApplyAsset(h, a, hLog)
		if err != nil {
			return err
		}
	}

	hLog.Info("hiveadmission components reconciled successfully")

	return nil
}
