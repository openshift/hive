package hive

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveconstants "github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
	logrusutil "github.com/openshift/hive/pkg/util/logrus"

	configv1 "github.com/openshift/api/config/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/operator/configobserver/apiserver"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"

	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/cache"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

const (
	clusterVersionCRDName              = "clusterversions.config.openshift.io"
	hiveAdmissionServingCertSecretName = "hiveadmission-serving-cert"
	kasCACertConfigMapName             = "kube-root-ca.crt"
	serviceCACertConfigMapName         = "openshift-service-ca.crt"
)

const (
	aggregatorClientCAHashAnnotation = "hive.openshift.io/ca-hash"
	servingCertSecretHashAnnotation  = "hive.openshift.io/serving-cert-secret-hash"
)

const (
	inputHashAnnotation = "hive.openshift.io/hive-admission-input-sources-hash"
)

var webhookAssets = []string{
	"config/hiveadmission/clusterdeployment-webhook.yaml",
	"config/hiveadmission/clusterimageset-webhook.yaml",
	"config/hiveadmission/clusterprovision-webhook.yaml",
	"config/hiveadmission/dnszones-webhook.yaml",
	"config/hiveadmission/machinepool-webhook.yaml",
	"config/hiveadmission/syncset-webhook.yaml",
	"config/hiveadmission/selectorsyncset-webhook.yaml",
}

func (r *ReconcileHiveConfig) deployHiveAdmission(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, namespacesToClean []string, additionalHashes ...string) error {
	deploymentAsset := "config/hiveadmission/deployment.yaml"
	namespacedAssets := []string{
		"config/hiveadmission/service.yaml",
		"config/hiveadmission/service-account.yaml",
	}
	// In OpenShift, we get the service and kube root CA certs from automatically-generated ConfigMaps
	if !r.isOpenShift {
		namespacedAssets = append(namespacedAssets,
			// This secret was automatically generated prior to k8s 1.24. We're including it to cover later versions.
			// Note that overwriting it should have no effect as k8s will still populate it for us.
			// Also note that this secret will be deleted automatically when the serviceaccount is deleted; our deletion
			// is redundant, but harmless.
			"config/hiveadmission/sa-token-secret.yaml",
		)
	}
	// Delete the assets from previous target namespaces
	assetsToClean := append(namespacedAssets, deploymentAsset)
	for _, ns := range namespacesToClean {
		for _, asset := range assetsToClean {
			hLog.Infof("Deleting asset %s from old target namespace %s", asset, ns)
			// DeleteAssetWithNSOverride already no-ops for IsNotFound
			if err := deleteAssetByPathWithNSOverride(h, asset, ns, instance); err != nil {
				return errors.Wrapf(err, "error deleting asset %s from old target namespace %s", asset, ns)
			}
		}
	}

	hiveNSName := GetHiveNamespace(instance)

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	for _, assetPath := range namespacedAssets {
		if _, err := applyRuntimeObject(h, fromAssetPath(assetPath), hLog, withNamespaceOverride(hiveNSName), withGarbageCollection(instance)); err != nil {
			hLog.WithError(err).Error("error applying object with namespace override")
			return err
		}
		hLog.WithField("asset", assetPath).Info("applied asset with namespace override")
	}

	// Apply global non-namespaced assets:
	applyAssets := []string{
		"config/hiveadmission/hiveadmission_rbac_role.yaml",
	}
	for _, a := range applyAssets {
		if _, err := applyRuntimeObject(h, fromAssetPath(a), hLog, withGarbageCollection(instance)); err != nil {
			return err
		}
	}

	// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
	clusterRoleBindingAssets := []string{
		"config/hiveadmission/hiveadmission_rbac_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {
		if _, err := applyRuntimeObject(h, crbFromAssetPath(crbAsset), hLog, crbWithSubjectNSOverride(hiveNSName), withGarbageCollection(instance)); err != nil {
			hLog.WithError(err).Error("error applying ClusterRoleBinding with namespace override")
			return err
		}
		hLog.WithField("asset", crbAsset).Info("applied ClusterRoleRoleBinding asset with namespace override")
	}

	asset := assets.MustAsset(deploymentAsset)
	hLog.Debug("reading deployment")
	hiveAdmDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveAdmContainer, err := containerByName(&hiveAdmDeployment.Spec.Template.Spec, "hiveadmission")
	if err != nil {
		return err
	}
	applyDeploymentConfig(instance, hivev1.DeploymentNameAdmission, hiveAdmContainer, hLog)

	hiveAdmDeployment.Namespace = hiveNSName
	if r.hiveImage != "" {
		hiveAdmContainer.Image = r.hiveImage
	}
	if r.hiveImagePullPolicy != "" {
		hiveAdmContainer.ImagePullPolicy = r.hiveImagePullPolicy
	}
	if hiveAdmDeployment.Annotations == nil {
		hiveAdmDeployment.Annotations = map[string]string{}
	}
	if hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations == nil {
		hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	hiveAdmDeployment.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash
	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[aggregatorClientCAHashAnnotation] = instance.Status.AggregatorClientCAHash

	httpProxy, httpsProxy, noProxy, err := r.discoverProxyVars()
	if err != nil {
		return err
	}
	controllerutils.SetProxyEnvVars(&hiveAdmDeployment.Spec.Template.Spec, httpProxy, httpsProxy, noProxy)

	// Include the proxy vars in the hash so we redeploy if they change
	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[inputHashAnnotation] = computeHash(
		httpProxy+httpsProxy+noProxy, additionalHashes...)

	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, managedDomainsConfigMapInfo, hiveAdmContainer)
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, awsPrivateLinkConfigMapInfo, hiveAdmContainer)
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, privateLinkConfigMapInfo, hiveAdmContainer)
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, r.supportedContractsConfigMapInfo(hLog), hiveAdmContainer)
	addReleaseImageVerificationConfigMapEnv(hiveAdmContainer, instance)

	if err := r.populateTLSConfig(hiveAdmContainer, hLog); err != nil {
		return err
	}

	validatingWebhooks := make([]*admregv1.ValidatingWebhookConfiguration, len(webhookAssets))
	for i, yaml := range webhookAssets {
		validatingWebhooks[i] = readRuntimeObjectOrDie[*admregv1.ValidatingWebhookConfiguration](
			admregv1.SchemeGroupVersion, assets.MustAsset(yaml))
	}

	hLog.Debug("reading apiservice")
	apiService := readRuntimeObjectOrDie[*apiregistrationv1.APIService](
		apiregistrationv1.SchemeGroupVersion, assets.MustAsset("config/hiveadmission/apiservice.yaml"))
	apiService.Spec.Service.Namespace = hiveNSName

	err = r.injectCerts(apiService, validatingWebhooks, nil, hiveNSName, hLog)
	if err != nil {
		hLog.WithError(err).Error("error injecting certs")
		return err
	}

	// Set the serving cert CA secret hash as an annotation on the pod template to force a rollout in the event it changes:
	servingCertSecret, err := r.hiveSecretLister.Secrets(hiveNSName).Get(hiveAdmissionServingCertSecretName)
	if err != nil {
		hLog.WithError(err).WithField("secretName", hiveAdmissionServingCertSecretName).Log(
			controllerutils.LogLevel(err), "error getting serving cert secret")
		return err
	}
	hLog.Info("Hashing serving cert secret onto a hiveadmission deployment annotation")
	certSecretHash := computeHash(servingCertSecret.Data)
	if hiveAdmDeployment.Spec.Template.Annotations == nil {
		hiveAdmDeployment.Spec.Template.Annotations = map[string]string{}
	}
	hiveAdmDeployment.Spec.Template.Annotations[servingCertSecretHashAnnotation] = certSecretHash

	// Apply shared pod config passed through from the operator deployment
	hiveAdmDeployment.Spec.Template.Spec.NodeSelector = r.sharedPodConfig.NodeSelector
	hiveAdmDeployment.Spec.Template.Spec.Tolerations = r.sharedPodConfig.Tolerations
	if ref := getImagePullSecretReference(instance); ref != nil {
		hiveAdmDeployment.Spec.Template.Spec.ImagePullSecrets = append(hiveAdmDeployment.Spec.Template.Spec.ImagePullSecrets, *ref)
	}

	result, err := applyRuntimeObject(h, passthrough(hiveAdmDeployment), hLog, withGarbageCollection(instance))
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.WithField("result", result).Info("hiveadmission deployment applied")

	result, err = applyRuntimeObject(h, passthrough(apiService), hLog, withGarbageCollection(instance))
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}
	hLog.Infof("apiservice applied (%s)", result)

	for _, webhook := range validatingWebhooks {
		result, err = applyRuntimeObject(h, passthrough(webhook), hLog, withGarbageCollection(instance))
		if err != nil {
			hLog.WithField("webhook", webhook.Name).WithError(err).Errorf("error applying validating webhook")
			return err
		}
		hLog.WithField("webhook", webhook.Name).Infof("validating webhook: %s", result)
	}

	hLog.Info("hiveadmission components reconciled successfully")
	return nil
}

// directAPIServerLister implements apiserver.APIServerLister by making direct API calls
type directAPIServerLister struct {
	reconciler *ReconcileHiveConfig
}

// PreRunHasSynced implements configobserver.Listers, minimally, because we're not caching.
func (d *directAPIServerLister) PreRunHasSynced() []cache.InformerSynced {
	return []cache.InformerSynced{}
}

// ResourceSyncer implements configobserver.Listers, but not really, because we're not caching.
func (d *directAPIServerLister) ResourceSyncer() resourcesynccontroller.ResourceSyncer {
	return nil
}

func (d *directAPIServerLister) APIServerLister() configlistersv1.APIServerLister {
	return &directLister{reconciler: d.reconciler}
}

// directLister implements configlistersv1.APIServerLister with direct API calls via reconciler
type directLister struct {
	reconciler *ReconcileHiveConfig
}

func (d *directLister) List(selector labels.Selector) ([]*configv1.APIServer, error) {
	list := &configv1.APIServerList{}
	if err := d.reconciler.List(context.TODO(), list, "", metav1.ListOptions{}); err != nil {
		return nil, err
	}
	result := make([]*configv1.APIServer, len(list.Items))
	for i := range list.Items {
		result[i] = &list.Items[i]
	}
	return result, nil
}

func (d *directLister) Get(name string) (*configv1.APIServer, error) {
	apiServer := &configv1.APIServer{}
	if err := d.reconciler.Get(context.TODO(), types.NamespacedName{Name: name}, apiServer); err != nil {
		return nil, err
	}
	return apiServer, nil
}

func (r *ReconcileHiveConfig) populateTLSConfig(hiveAdmContainer *corev1.Container, hLog log.FieldLogger) error {
	if !r.isOpenShift {
		return nil
	}
	observedConfig, errs := apiserver.ObserveTLSSecurityProfileToArguments(
		&directAPIServerLister{reconciler: r},
		logrusutil.NewLoggingEventRecorder(hLog, "hiveadmission-tls-config"),
		map[string]interface{}{})

	if len(errs) > 0 {
		return errors.Wrap(utilerrors.NewAggregate(errs), "failed to discover global TLS config from APIServer cluster")
	}

	hLog.WithField("config", observedConfig).Debug("observed TLS config")

	if len(observedConfig) == 0 {
		return errors.New("observed TLS config was empty")
	}

	asa, tlsmvk, tlscsk := "apiServerArguments", "tls-min-version", "tls-cipher-suites"

	tlsmv, found, err := unstructured.NestedString(observedConfig, asa, tlsmvk)
	if !found || err != nil {
		return errors.Wrapf(err, "could not find %s.%s in observed TLS config %v", asa, tlsmvk, observedConfig)
	}
	tlscs, found, err := unstructured.NestedStringSlice(observedConfig, asa, tlscsk)
	if !found || err != nil {
		return errors.Wrapf(err, "could not find %s.%s in observed TLS config %v", asa, tlscsk, observedConfig)
	}

	// NOTE: These arguments (--tls-min-version, --tls-cipher-suites) are expected to be *absent*
	// from the container we're given.
	hiveAdmContainer.Command = append(
		hiveAdmContainer.Command,
		fmt.Sprintf("--%s=%s", tlsmvk, tlsmv),
		fmt.Sprintf("--%s=%s", tlscsk, strings.Join(tlscs, ",")),
	)

	return nil
}

// Modern OpenShift injects two ConfigMaps into every namespace. One contains the service CA cert;
// the other the kube root CA cert.
func (r *ReconcileHiveConfig) getCACertsOpenShift(hLog log.FieldLogger, hiveNSName string) ([]byte, []byte, error) {
	kasCACertConfigMap := corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: hiveNSName, Name: kasCACertConfigMapName}, &kasCACertConfigMap); err != nil {
		hLog.WithError(err).Errorf("error getting %s configmap in hive namespace", kasCACertConfigMapName)
		return nil, nil, err
	}
	kubeCAstr, ok := kasCACertConfigMap.Data["ca.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("configmap %s did not contain key ca.crt", kasCACertConfigMapName)
	}

	svcCACertConfigMap := corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: hiveNSName, Name: serviceCACertConfigMapName}, &svcCACertConfigMap); err != nil {
		hLog.WithError(err).Errorf("error getting %s configmap in hive namespace", serviceCACertConfigMapName)
		return nil, nil, err
	}
	svcCAstr, ok := svcCACertConfigMap.Data["service-ca.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("configmap %s did not contain key service-ca.crt", serviceCACertConfigMapName)
	}

	return []byte(svcCAstr), []byte(kubeCAstr), nil
}

// If we're running on vanilla Kube (mostly devs using kind), we will not have access to the
// ConfigMaps injected by OpenShift. Look up the certs in the Secrets created via
// hack/hiveadmission-dev-cert.sh. (TODO: automate -- see HIVE-1449.)
func (r *ReconcileHiveConfig) getCACertsNonOpenShift(hLog log.FieldLogger, hiveNSName string) ([]byte, []byte, error) {
	// Locate the kube CA by looking up secrets in hive namespace, finding one of
	// type 'kubernetes.io/service-account-token', and reading the CA off it.
	hLog.Debugf("listing secrets in %s namespace", hiveNSName)
	secrets, err := r.hiveSecretLister.Secrets(hiveNSName).List(labels.Everything())
	if err != nil {
		hLog.WithError(err).Error("error listing secrets in hive namespace")
		return nil, nil, err
	}
	var firstSATokenSecret *corev1.Secret
	hLog.Debugf("found %d secrets", len(secrets))
	for _, s := range secrets {
		if s.Type == corev1.SecretTypeServiceAccountToken {
			firstSATokenSecret = s
			break
		}
	}
	if firstSATokenSecret == nil {
		return nil, nil, fmt.Errorf("no %s secrets found", corev1.SecretTypeServiceAccountToken)
	}
	kubeCA, ok := firstSATokenSecret.Data["ca.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("secret %s did not contain key ca.crt", firstSATokenSecret.Name)
	}

	// Load the service CA:
	serviceCA, ok := firstSATokenSecret.Data["service-ca.crt"]
	if !ok {
		hLog.Warnf("secret %s did not contain key service-ca.crt, likely not running on OpenShift, using ca.crt instead", firstSATokenSecret.Name)
		serviceCA = kubeCA
	}

	return serviceCA, kubeCA, nil
}

func (r *ReconcileHiveConfig) injectCerts(apiService *apiregistrationv1.APIService, validatingWebhooks []*admregv1.ValidatingWebhookConfiguration, mutatingWebhooks []*admregv1.MutatingWebhookConfiguration, hiveNS string, hLog log.FieldLogger) error {
	var serviceCA, kubeCA []byte
	var err error
	hLog.Debug("modifying hiveadmission webhooks for CA certs")
	if r.isOpenShift {
		hLog.Debug("OpenShift cluster detected")
		serviceCA, kubeCA, err = r.getCACertsOpenShift(hLog, hiveNS)
	} else {
		hLog.Debug("non-OpenShift cluster detected")
		serviceCA, kubeCA, err = r.getCACertsNonOpenShift(hLog, hiveNS)
	}
	if err != nil {
		return err
	}
	hLog.WithField("kubeCA", string(kubeCA)).WithField("serviceCA", string(serviceCA)).Debugf("found CA certs")

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

func addReleaseImageVerificationConfigMapEnv(container *corev1.Container, instance *hivev1.HiveConfig) {
	if instance.Spec.ReleaseImageVerificationConfigMapRef == nil {
		return
	}
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  hiveconstants.HiveReleaseImageVerificationConfigMapNamespaceEnvVar,
		Value: instance.Spec.ReleaseImageVerificationConfigMapRef.Namespace,
	}, corev1.EnvVar{
		Name:  hiveconstants.HiveReleaseImageVerificationConfigMapNameEnvVar,
		Value: instance.Spec.ReleaseImageVerificationConfigMapRef.Name,
	})
}
