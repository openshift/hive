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

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHiveConfig) deployHiveAdmission(haLog log.FieldLogger, instance *hivev1.HiveConfig, recorder events.Recorder) error {
	asset := assets.MustAsset("config/hiveadmission/service.yaml")
	haLog.Debug("reading service")
	hiveAdmService := resourceread.ReadServiceV1OrDie(asset)

	asset = assets.MustAsset("config/hiveadmission/service-account.yaml")
	haLog.Debug("reading service account")
	hiveAdmServiceAcct := resourceread.ReadServiceAccountV1OrDie(asset)

	asset = assets.MustAsset("config/hiveadmission/daemonset.yaml")
	haLog.Debug("reading daemonset")
	hiveAdmDaemonSet := resourceread.ReadDaemonSetV1OrDie(asset)

	asset = assets.MustAsset("config/hiveadmission/apiservice.yaml")
	haLog.Debug("reading apiservice")
	hiveAdmAPIService := util.ReadAPIServiceV1Beta1OrDie(asset, r.scheme)

	asset = assets.MustAsset("config/hiveadmission/dnszones-webhook.yaml")
	haLog.Debug("reading DNSZones webhook")
	dnsZonesWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	asset = assets.MustAsset("config/hiveadmission/clusterdeployment-webhook.yaml")
	haLog.Debug("reading ClusterDeployment webhook")
	cdWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	asset = assets.MustAsset("config/hiveadmission/clusterimageset-webhook.yaml")
	haLog.Debug("reading ClusterImageSet webhook")
	cisWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	// Set owner refs on all objects in the deployment so deleting the operator CRD
	// will clean everything up:
	if err := controllerutil.SetControllerReference(instance, hiveAdmService, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return err
	}

	if err := controllerutil.SetControllerReference(instance, hiveAdmServiceAcct, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return err
	}

	if err := controllerutil.SetControllerReference(instance, hiveAdmDaemonSet, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return err
	}

	_, changed, err := resourceapply.ApplyService(r.kubeClient.CoreV1(), recorder, hiveAdmService)
	if err != nil {
		haLog.WithError(err).Error("error applying service")
		return err
	}
	haLog.WithField("changed", changed).Info("service updated")

	_, changed, err = resourceapply.ApplyServiceAccount(r.kubeClient.CoreV1(), recorder, hiveAdmServiceAcct)
	if err != nil {
		haLog.WithError(err).Error("error applying service account")
		return err
	}
	haLog.WithField("changed", changed).Info("service account updated")

	expectedDSGen := int64(0)
	currentDS := &appsv1.DaemonSet{}
	foundCurrentDS := false
	err = r.Get(context.Background(), types.NamespacedName{Name: hiveAdmDaemonSet.Name, Namespace: hiveAdmDaemonSet.Namespace}, currentDS)
	if err != nil && !errors.IsNotFound(err) {
		haLog.WithError(err).Error("error looking up current daemonset")
		return err
	} else if err == nil {
		expectedDSGen = currentDS.ObjectMeta.Generation
		foundCurrentDS = true
	}

	if r.hiveImage != "" {
		hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image = r.hiveImage
	}

	// ApplyDaemonSet does not check much of the Spec for changes. Do some manual
	// checking and if we see something we care about has changed, force an update
	// by changing the expected deployment generation.
	if foundCurrentDS && currentDS.Spec.Template.Spec.Containers[0].Image !=
		hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image {
		haLog.WithFields(log.Fields{
			"current": currentDS.Spec.Template.Spec.Containers[0].Image,
			"new":     hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image,
		}).Info("overriding daemonset image")
		expectedDSGen = expectedDSGen - 1
	}

	_, changed, err = resourceapply.ApplyDaemonSet(r.kubeClient.AppsV1(), recorder, hiveAdmDaemonSet, expectedDSGen, false)
	if err != nil {
		haLog.WithError(err).Error("error applying daemonset")
		return err
	}
	haLog.WithField("changed", changed).Info("daemonset updated")

	_, changed, err = resourceapply.ApplyAPIService(r.apiregClient, hiveAdmAPIService)
	if err != nil {
		haLog.WithError(err).Error("error applying apiservice")
		return err
	}
	haLog.WithField("changed", changed).Info("apiservice updated")

	_, changed, err = util.ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), dnsZonesWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying DNSZones webhook")
		return err
	}
	haLog.WithField("changed", changed).Info("DNSZones webhook updated")

	_, changed, err = util.ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), cdWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying ClusterDeployment webhook")
		return err
	}
	haLog.WithField("changed", changed).Info("ClusterDeployment webhook updated")

	_, changed, err = util.ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), cisWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying ClusterImageSet webhook")
		return err
	}
	haLog.WithField("changed", changed).Info("ClusterImageSet webhook updated")

	haLog.Info("HiveAdmissionConfig components reconciled")

	return nil
}
