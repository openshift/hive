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

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/hive/pkg/operator/assets"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileHiveConfig) deployHiveAdmission(haLog log.FieldLogger, instance *hivev1alpha1.HiveConfig, recorder events.Recorder) error {
	asset, err := assets.Asset("config/hiveadmission/service.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading service")
	hiveAdmService := resourceread.ReadServiceV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/service-account.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading service account")
	hiveAdmServiceAcct := resourceread.ReadServiceAccountV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/daemonset.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading daemonset")
	hiveAdmDaemonSet := resourceread.ReadDaemonSetV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/apiservice.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading apiservice")
	hiveAdmAPIService := ReadAPIServiceV1Beta1OrDie(asset, r.scheme)

	asset, err = assets.Asset("config/hiveadmission/dnszones-webhook.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading DNSZones webhook")
	dnsZonesWebhook := ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	asset, err = assets.Asset("config/hiveadmission/clusterdeployment-webhook.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return err
	}
	haLog.Debug("reading ClusterDeployment webhook")
	cdWebhook := ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

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
	} else {
		haLog.WithField("changed", changed).Info("service updated")
	}

	_, changed, err = resourceapply.ApplyServiceAccount(r.kubeClient.CoreV1(), recorder, hiveAdmServiceAcct)
	if err != nil {
		haLog.WithError(err).Error("error applying service account")
		return err
	} else {
		haLog.WithField("changed", changed).Info("service account updated")
	}

	expectedDSGen := int64(0)
	currentDS := &appsv1.DaemonSet{}
	err = r.Get(context.Background(), types.NamespacedName{Name: hiveAdmDaemonSet.Name, Namespace: hiveAdmDaemonSet.Namespace}, currentDS)
	if err != nil && !errors.IsNotFound(err) {
		haLog.WithError(err).Error("error looking up current daemonset")
		return err
	} else if err == nil {
		expectedDSGen = currentDS.ObjectMeta.Generation
	}

	if instance.Spec.Image != "" {
		haLog.WithFields(log.Fields{
			"orig": hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image,
			"new":  instance.Spec.Image,
		}).Info("overriding deployment image")
		hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image
	}
	haLog.WithFields(log.Fields{
		"orig": hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image,
	}).Info("did it work?")

	_, changed, err = resourceapply.ApplyDaemonSet(r.kubeClient.AppsV1(), recorder, hiveAdmDaemonSet, expectedDSGen, false)
	if err != nil {
		haLog.WithError(err).Error("error applying daemonset")
		return err
	} else {
		haLog.WithField("changed", changed).Info("daemonset updated")
	}

	_, changed, err = resourceapply.ApplyAPIService(r.apiregClient, hiveAdmAPIService)
	if err != nil {
		haLog.WithError(err).Error("error applying apiservice")
		return err
	} else {
		haLog.WithField("changed", changed).Info("apiservice updated")
	}

	_, changed, err = ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), dnsZonesWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying DNSZones webhook")
		return err
	} else {
		haLog.WithField("changed", changed).Info("DNSZones webhook updated")
	}

	_, changed, err = ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), cdWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying ClusterDeployment webhook")
		return err
	} else {
		haLog.WithField("changed", changed).Info("ClusterDeployment webhook updated")
	}

	haLog.Info("HiveAdmissionConfig components reconciled")

	return nil
}
