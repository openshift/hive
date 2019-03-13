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
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	admissionDaemonSetName = "hiveadmission"
)

func (r *ReconcileHiveConfig) deployHiveAdmission(haLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	asset := assets.MustAsset("config/hiveadmission/daemonset.yaml")
	haLog.Debug("reading daemonset")
	hiveAdmDaemonSet := resourceread.ReadDaemonSetV1OrDie(asset)

	err := util.ApplyAsset(h, "config/hiveadmission/service.yaml", haLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/hiveadmission/service-account.yaml", haLog)
	if err != nil {
		return err
	}

	if r.hiveImage != "" {
		hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image = r.hiveImage
	}

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
		scheme.Scheme)

	buf := bytes.NewBuffer([]byte{})
	err = s.Encode(hiveAdmDaemonSet, buf)
	if err != nil {
		haLog.WithError(err).Error("error encoding daemonset")
		return err
	}
	err = h.Apply(buf.Bytes())
	if err != nil {
		haLog.WithError(err).Error("error applying daemonset")
		return err
	}
	haLog.Info("daemonset applied")

	err = util.ApplyAsset(h, "config/hiveadmission/apiservice.yaml", haLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/hiveadmission/clusterdeployment-webhook.yaml", haLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/hiveadmission/clusterimageset-webhook.yaml", haLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/hiveadmission/dnszones-webhook.yaml", haLog)
	if err != nil {
		return err
	}

	haLog.Info("hiveadmission components reconciled successfully")

	return nil
}

func (r *ReconcileHiveConfig) redeployHiveAdmission(haLog log.FieldLogger, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	daemonSet, err := r.kubeClient.AppsV1().DaemonSets(hiveNamespace).Get(admissionDaemonSetName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// Daemonset doesn't exist yet, nothing to do
		return nil
	}
	if err != nil {
		haLog.WithError(err).Error("cannot retrieve admission daemonset")
		return err
	}
	_, _, err = resourceapply.ApplyDaemonSet(r.kubeClient.AppsV1(), recorder, daemonSet, daemonSet.Generation, true)
	if err != nil {
		haLog.WithError(err).Error("cannot force daemonset redeploy")
		return err
	}
	return nil
}
