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
	"fmt"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	admregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterVersionCRDName = "clusterversions.config.openshift.io"
)

const (
	aggregatorClientCAHashAnnotation = "hive.openshift.io/ca-hash"
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
	if hiveAdmDeployment.Annotations == nil {
		hiveAdmDeployment.Annotations = map[string]string{}
	}
	if hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations == nil {
		hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	hiveAdmDeployment.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash
	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
		scheme.Scheme)

	err = h.ApplyRuntimeObject(hiveAdmDeployment, s)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Info("deployment applied")

	hLog.Debug("reading apiservice")
	asset = assets.MustAsset("config/hiveadmission/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme.Scheme)

	asset = assets.MustAsset("config/hiveadmission/clusterdeployment-webhook.yaml")
	cdWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, scheme.Scheme)

	asset = assets.MustAsset("config/hiveadmission/clusterimageset-webhook.yaml")
	cisWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, scheme.Scheme)

	asset = assets.MustAsset("config/hiveadmission/dnszones-webhook.yaml")
	dnsZonesWebhook := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, scheme.Scheme)

	// If on 3.11 we need to set the service CA on the apiservice.
	is311, err := r.is311(hLog)
	if err != nil {
		hLog.Error("error detecting 3.11 cluster")
		return err
	}
	if is311 {
		hLog.Debug("3.11 cluster detected, modifying objects for CA certs")
		err = r.injectCerts(apiService, []*admregv1.ValidatingWebhookConfiguration{cdWebhook, cisWebhook, dnsZonesWebhook}, hLog)
		if err != nil {
			hLog.WithError(err).Error("error injecting certs")
			return err
		}
	}

	err = h.ApplyRuntimeObject(apiService, s)
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}
	hLog.Info("apiservice applied")

	err = h.ApplyRuntimeObject(cdWebhook, s)
	if err != nil {
		hLog.WithError(err).Error("error applying cluster deployment webhook")
		return err
	}
	hLog.Info("cluster deployment webhook applied")

	err = h.ApplyRuntimeObject(cisWebhook, s)
	if err != nil {
		hLog.WithError(err).Error("error applying cluster image set webhook")
		return err
	}
	hLog.Info("cluster image set webhook applied")

	err = h.ApplyRuntimeObject(dnsZonesWebhook, s)
	if err != nil {
		hLog.WithError(err).Error("error applying dns zones webhook")
		return err
	}
	hLog.Info("dns zones webhook applied")

	hLog.Info("hiveadmission components reconciled successfully")

	return nil
}

func (r *ReconcileHiveConfig) injectCerts(apiService *apiregistrationv1.APIService, webhooks []*admregv1.ValidatingWebhookConfiguration, hLog log.FieldLogger) error {

	// Locate the kube CA by looking up secrets in hive namespace, finding one of
	// type 'kubernetes.io/service-account-token', and reading the CA off it.
	hLog.Debug("listing secrets in hive namespace")
	secrets := &corev1.SecretList{}
	err := r.Client.List(context.Background(), &client.ListOptions{Namespace: hiveNamespace}, secrets)
	if err != nil {
		hLog.WithError(err).Error("error listing secrets in hive namespace")
		return err
	}
	var firstSATokenSecret *corev1.Secret
	hLog.Debugf("found %d secrets", len(secrets.Items))
	for _, s := range secrets.Items {
		if s.Type == corev1.SecretTypeServiceAccountToken {
			firstSATokenSecret = &s
			break
		}
	}
	if firstSATokenSecret == nil {
		return fmt.Errorf("no %s secrets found", corev1.SecretTypeServiceAccountToken)
	}
	kubeCA, ok := firstSATokenSecret.Data["ca.crt"]
	if !ok {
		return fmt.Errorf("secret %s did not contain key ca.crt", firstSATokenSecret.Name)
	}
	hLog.Debugf("found kube CA: %s", string(kubeCA))

	// Load the service CA:
	serviceCA, ok := firstSATokenSecret.Data["service-ca.crt"]
	if !ok {
		return fmt.Errorf("secret %s did not contain key service-ca.crt", firstSATokenSecret.Name)
	}
	hLog.Debugf("found service CA: %s", string(serviceCA))

	// Add the service CA to the aggregated API service:
	apiService.Spec.CABundle = serviceCA

	// Add the kube CA to each webhook:
	for whi := range webhooks {
		for whwhi := range webhooks[whi].Webhooks {
			webhooks[whi].Webhooks[whwhi].ClientConfig.CABundle = kubeCA
		}
	}

	return nil
}

// is311 returns true if this is a 3.11 OpenShift cluster. We check by looking for a ClusterVersion CRD,
// which should only exist on OpenShift 4.x. We do not expect Hive to ever be deployed on pre-3.11.
func (r *ReconcileHiveConfig) is311(hLog log.FieldLogger) (bool, error) {
	cvCRD := &apiextv1beta1.CustomResourceDefinition{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: clusterVersionCRDName}, cvCRD)
	if err != nil && errors.IsNotFound(err) {
		// If this CRD does not exist, we must not be on a 4.x cluster.
		return true, nil
	} else if err != nil {
		hLog.WithError(err).Error("error fetching clusterversion CRD")
		return false, err
	}
	return false, nil
}
