package hive

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	hiveconstants "github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/contracts"
	"github.com/openshift/hive/pkg/util/scheme"

	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

const (
	clusterVersionCRDName              = "clusterversions.config.openshift.io"
	hiveAdmissionServingCertSecretName = "hiveadmission-serving-cert"
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
		// This secret was automatically generated prior to k8s 1.24. We're including it to cover later versions.
		// Note that overwriting it should have no effect as k8s will still populate it for us.
		// Also note that this secret will be deleted automatically when the serviceaccount is deleted; our deletion
		// is redundant, but harmless.
		"config/hiveadmission/sa-token-secret.yaml",
	}
	// Delete the assets from previous target namespaces
	assetsToClean := append(namespacedAssets, deploymentAsset)
	for _, ns := range namespacesToClean {
		for _, asset := range assetsToClean {
			hLog.Infof("Deleting asset %s from old target namespace %s", asset, ns)
			// DeleteAssetWithNSOverride already no-ops for IsNotFound
			if err := util.DeleteAssetWithNSOverride(h, asset, ns, instance); err != nil {
				return errors.Wrapf(err, "error deleting asset %s from old target namespace %s", asset, ns)
			}
		}
	}

	hiveNSName := GetHiveNamespace(instance)

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	for _, assetPath := range namespacedAssets {
		if err := util.ApplyAssetWithNSOverrideAndGC(h, assetPath, hiveNSName, instance); err != nil {
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
		if err := util.ApplyAssetWithGC(h, a, instance, hLog); err != nil {
			return err
		}
	}

	// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
	clusterRoleBindingAssets := []string{
		"config/hiveadmission/hiveadmission_rbac_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {
		if err := util.ApplyClusterRoleBindingAssetWithSubjectNSOverrideAndGC(h, crbAsset, hiveNSName, instance); err != nil {
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
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, r.supportedContractsConfigMapInfo(), hiveAdmContainer)
	addReleaseImageVerificationConfigMapEnv(hiveAdmContainer, instance)

	scheme := scheme.GetScheme()

	validatingWebhooks := make([]*admregv1.ValidatingWebhookConfiguration, len(webhookAssets))
	for i, yaml := range webhookAssets {
		asset = assets.MustAsset(yaml)
		wh := util.ReadValidatingWebhookConfigurationV1OrDie(asset, scheme)
		validatingWebhooks[i] = wh
	}

	hLog.Debug("reading apiservice")
	asset = assets.MustAsset("config/hiveadmission/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme)
	apiService.Spec.Service.Namespace = hiveNSName

	// If we're running on vanilla Kube (mostly devs using kind), we
	// will not have access to the service cert injection we normally use. Lookup
	// the cluster CA and inject into the webhooks.
	// NOTE: If this is vanilla kube, you will also need to manually create a certificate
	// secret, see hack/hiveadmission-dev-cert.sh. (TODO: automate -- see HIVE-1449.)
	if !r.isOpenShift {
		hLog.Debug("non-OpenShift 4.x cluster detected, modifying hiveadmission webhooks for CA certs")
		err = r.injectCerts(apiService, validatingWebhooks, nil, hiveNSName, hLog)
		if err != nil {
			hLog.WithError(err).Error("error injecting certs")
			return err
		}
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

	// Apply nodeSelector and tolerations passed through from the operator deployment
	hiveAdmDeployment.Spec.Template.Spec.NodeSelector = r.nodeSelector
	hiveAdmDeployment.Spec.Template.Spec.Tolerations = r.tolerations

	result, err := util.ApplyRuntimeObjectWithGC(h, hiveAdmDeployment, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.WithField("result", result).Info("hiveadmission deployment applied")

	result, err = util.ApplyRuntimeObjectWithGC(h, apiService, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}
	hLog.Infof("apiservice applied (%s)", result)

	for _, webhook := range validatingWebhooks {
		result, err = util.ApplyRuntimeObjectWithGC(h, webhook, instance)
		if err != nil {
			hLog.WithField("webhook", webhook.Name).WithError(err).Errorf("error applying validating webhook")
			return err
		}
		hLog.WithField("webhook", webhook.Name).Infof("validating webhook: %s", result)
	}

	hLog.Info("hiveadmission components reconciled successfully")
	return nil
}

func (r *ReconcileHiveConfig) getCACerts(hLog log.FieldLogger, hiveNSName string) ([]byte, []byte, error) {
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
	hLog.Debugf("found kube CA: %s", string(kubeCA))

	// Load the service CA:
	serviceCA, ok := firstSATokenSecret.Data["service-ca.crt"]
	if !ok {
		hLog.Warnf("secret %s did not contain key service-ca.crt, likely not running on OpenShift, using ca.crt instead", firstSATokenSecret.Name)
		serviceCA = kubeCA
	}

	hLog.Debugf("found service CA: %s", string(serviceCA))
	return serviceCA, kubeCA, nil
}

func (r *ReconcileHiveConfig) injectCerts(apiService *apiregistrationv1.APIService, validatingWebhooks []*admregv1.ValidatingWebhookConfiguration, mutatingWebhooks []*admregv1.MutatingWebhookConfiguration, hiveNS string, hLog log.FieldLogger) error {
	serviceCA, kubeCA, err := r.getCACerts(hLog, hiveNS)
	if err != nil {
		return err
	}

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

// allowedContracts is the list of operator whitelisted contracts that hive will accept
// from CRDs.
var allowedContracts = sets.NewString(
	hivecontractsv1alpha1.ClusterInstallContractLabelKey,
)

// knowContracts is a list of contracts and their implementations that doesn't
// require discovery using CRDs
var knowContracts = contracts.SupportedContractImplementationsList{{
	Name: hivecontractsv1alpha1.ClusterInstallContractName,
	Supported: []contracts.ContractImplementation{{
		Group:   "extensions.hive.openshift.io",
		Version: "v1beta1",
		Kind:    "AgentClusterInstall",
	}},
}}

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
