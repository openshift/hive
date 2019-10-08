package hive

import (
	"bytes"
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	webhooks "github.com/openshift/hive/pkg/apis/hive/v1alpha1/validating-webhooks"
	"github.com/openshift/hive/pkg/constants"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	admregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterVersionCRDName       = "clusterversions.config.openshift.io"
	managedDomainsConfigMapName = "managed-domains"
)

const (
	aggregatorClientCAHashAnnotation           = "hive.openshift.io/ca-hash"
	deprecatedClusterDeploymentMutatingWebhook = "mutateclusterdeployments.admission.hive.openshift.io"
)

var (
	mutatingWebhookConfigurationResource = schema.GroupVersionResource{
		Group:    "admissionregistration.k8s.io",
		Version:  "v1beta1",
		Resource: "mutatingwebhookconfigurations",
	}
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
	if r.hiveImagePullPolicy != "" {
		hiveAdmDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = r.hiveImagePullPolicy
	}
	if hiveAdmDeployment.Annotations == nil {
		hiveAdmDeployment.Annotations = map[string]string{}
	}
	if hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations == nil {
		hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	hiveAdmDeployment.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash
	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash

	if len(instance.Spec.ManagedDomains) > 0 {
		configMap := managedDomainsConfigMap(hiveAdmDeployment.Namespace, instance.Spec.ManagedDomains)
		_, err = h.ApplyRuntimeObject(configMap, scheme.Scheme)
		if err != nil {
			hLog.WithError(err).Error("error applying managed domains configmap")
		}
		volume := corev1.Volume{}
		volume.Name = "managed-domains"
		volume.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: managedDomainsConfigMapName,
			},
		}
		volumeMount := corev1.VolumeMount{
			Name:      "managed-domains",
			MountPath: "/data/config",
		}
		envVar := corev1.EnvVar{
			Name:  webhooks.ManagedDomainsFileEnvVar,
			Value: "/data/config/domains",
		}
		hiveAdmDeployment.Spec.Template.Spec.Volumes = append(hiveAdmDeployment.Spec.Template.Spec.Volumes, volume)
		hiveAdmDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(hiveAdmDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, volumeMount)
		hiveAdmDeployment.Spec.Template.Spec.Containers[0].Env = append(hiveAdmDeployment.Spec.Template.Spec.Containers[0].Env, envVar)
	}

	result, err := h.ApplyRuntimeObject(hiveAdmDeployment, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("deployment applied (%s)", result)

	hLog.Debug("reading apiservice")
	asset = assets.MustAsset("config/hiveadmission/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme.Scheme)

	webhooks := map[string]runtime.Object{}
	validatingWebhooks := []*admregv1.ValidatingWebhookConfiguration{}
	for _, yaml := range []string{
		"config/hiveadmission/clusterdeployment-webhook.yaml",
		"config/hiveadmission/clusterimageset-webhook.yaml",
		"config/hiveadmission/clusterprovision-webhook.yaml",
		"config/hiveadmission/dnszones-webhook.yaml",
		"config/hiveadmission/syncset-webhook.yaml",
		"config/hiveadmission/selectorsyncset-webhook.yaml",
	} {
		asset = assets.MustAsset(yaml)
		wh := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, scheme.Scheme)
		webhooks[yaml] = wh
		validatingWebhooks = append(validatingWebhooks, wh)
	}

	// If on 3.11 we need to set the service CA on the apiservice.
	is311, err := r.is311(hLog)
	if err != nil {
		hLog.Error("error detecting 3.11 cluster")
		return err
	}
	if is311 {
		hLog.Debug("3.11 cluster detected, modifying objects for CA certs")
		err = r.injectCerts(apiService, validatingWebhooks, nil, hLog)
		if err != nil {
			hLog.WithError(err).Error("error injecting certs")
			return err
		}
	}

	result, err = h.ApplyRuntimeObject(apiService, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}
	hLog.Infof("apiservice applied (%s)", result)

	for webhookFile, webhook := range webhooks {
		result, err = h.ApplyRuntimeObject(webhook, scheme.Scheme)
		if err != nil {
			hLog.WithError(err).Errorf("error applying validating webhook %q", webhookFile)
			return err
		}
		hLog.Infof("validating webhook %q applied (%s)", webhookFile, result)
	}

	if _, err = r.dynamicClient.Resource(mutatingWebhookConfigurationResource).Get(deprecatedClusterDeploymentMutatingWebhook, metav1.GetOptions{}); err == nil {
		err = r.dynamicClient.Resource(mutatingWebhookConfigurationResource).Delete(deprecatedClusterDeploymentMutatingWebhook, &metav1.DeleteOptions{})
		if err != nil {
			hLog.WithError(err).Error("error deleting deprecated mutating webhook configuration for cluster deployments")
			return err
		}
		hLog.Infof("deprecated mutating webhook configuration (%s) removed", deprecatedClusterDeploymentMutatingWebhook)
	}

	// Remove outdated validatingwebhookconfigurations
	removeValidatingWebhooks := []string{
		"clusterdeployments.admission.hive.openshift.io",
		"clusterimagesets.admission.hive.openshift.io",
		"dnszones.admission.hive.openshift.io",
		"selectorsyncsets.admission.hive.openshift.io",
		"syncsets.admission.hive.openshift.io",
	}
	for _, webhookName := range removeValidatingWebhooks {
		webhookConfig := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{}
		err := r.Get(context.Background(), types.NamespacedName{Name: webhookName}, webhookConfig)
		if err != nil && !errors.IsNotFound(err) {
			hLog.WithError(err).Error("error looking for obsolete ValidatingWebhookConfiguration")
			return err
		}
		if err == nil {
			err = r.Delete(context.Background(), webhookConfig)
			if err != nil {
				hLog.WithError(err).WithField("ValidatingWebhookConfiguration", webhookConfig).Error(
					"error deleting outdated ValidatingWebhookConfiguration")
				return err
			}
			hLog.WithField("ValidatingWebhookConfiguration", webhookName).Info("deleted outdated ValidatingWebhookConfiguration")
		}
	}

	hLog.Info("hiveadmission components reconciled successfully")
	return nil
}

func (r *ReconcileHiveConfig) injectCerts(apiService *apiregistrationv1.APIService, validatingWebhooks []*admregv1.ValidatingWebhookConfiguration, mutatingWebhooks []*admregv1.MutatingWebhookConfiguration, hLog log.FieldLogger) error {

	// Locate the kube CA by looking up secrets in hive namespace, finding one of
	// type 'kubernetes.io/service-account-token', and reading the CA off it.
	hLog.Debug("listing secrets in hive namespace")
	secrets := &corev1.SecretList{}
	err := r.Client.List(context.Background(), secrets, client.InNamespace(constants.HiveNamespace))
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

	// Add the kube CA to each validating webhook:
	for whi := range validatingWebhooks {
		for whwhi := range validatingWebhooks[whi].Webhooks {
			validatingWebhooks[whi].Webhooks[whwhi].ClientConfig.CABundle = kubeCA
		}
	}

	// Add the kube CA to each mutating webhook:
	for whi := range mutatingWebhooks {
		for whwhi := range mutatingWebhooks[whi].Webhooks {
			mutatingWebhooks[whi].Webhooks[whwhi].ClientConfig.CABundle = kubeCA
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

func managedDomainsConfigMap(namespace string, domains []string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	cm.Kind = "ConfigMap"
	cm.APIVersion = "v1"
	cm.Name = managedDomainsConfigMapName
	cm.Namespace = namespace
	domainsData := &bytes.Buffer{}
	for _, domain := range domains {
		fmt.Fprintf(domainsData, "%s\n", domain)
	}
	cm.Data = map[string]string{"domains": domainsData.String()}
	return cm
}
