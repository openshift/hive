package hive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/contracts"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	admregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	featureGateConfigMapName = "hive-feature-gates"
	inputHashAnnotation      = "hive.openshift.io/hive-admission-input-sources-hash"
)

const (
	supportedContractsConfigMapName      = "hive-supported-contracts"
	supportedContractsConfigMapNameKey   = "supported-contracts"
	supportedContractsConfigMapMountPath = "/data/supported-contracts-config"
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

func (r *ReconcileHiveConfig) deployHiveAdmission(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder, mdConfigMap *corev1.ConfigMap, additionalHashes ...string) error {
	hiveNSName := getHiveNamespace(instance)

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	namespacedAssets := []string{
		"config/hiveadmission/service.yaml",
		"config/hiveadmission/service-account.yaml",
	}
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

	asset := assets.MustAsset("config/hiveadmission/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveAdmDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveAdmDeployment.Namespace = hiveNSName
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

	hasher := md5.New()
	for _, v := range additionalHashes {
		hasher.Write([]byte(v))
	}
	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[inputHashAnnotation] = hex.EncodeToString(hasher.Sum(nil))
	controllerutils.SetProxyEnvVars(&hiveAdmDeployment.Spec.Template.Spec,
		os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("NO_PROXY"))

	addManagedDomainsVolume(&hiveAdmDeployment.Spec.Template.Spec, mdConfigMap.Name)
	addAWSPrivateLinkConfigVolume(&hiveAdmDeployment.Spec.Template.Spec)
	addSupportedContractsConfigVolume(&hiveAdmDeployment.Spec.Template.Spec)

	validatingWebhooks := make([]*admregv1.ValidatingWebhookConfiguration, len(webhookAssets))
	for i, yaml := range webhookAssets {
		asset = assets.MustAsset(yaml)
		wh := util.ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, scheme.Scheme)
		validatingWebhooks[i] = wh
	}

	hLog.Debug("reading apiservice")
	asset = assets.MustAsset("config/hiveadmission/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme.Scheme)
	apiService.Spec.Service.Namespace = hiveNSName

	// If on 3.11 we need to set the service CA on the apiservice.
	is311, err := r.is311(hLog)
	if err != nil {
		hLog.Error("error detecting 3.11 cluster")
		return err
	}
	// If we're running on vanilla Kube (mostly devs using kind), or OpenShift 3.x, we
	// will not have access to the service cert injection we normally use. Lookup
	// the cluster CA and inject into the webhooks.
	// NOTE: If this is vanilla kube, you will also need to manually create a certificate
	// secret, see hack/hiveadmission-dev-cert.sh.
	isOpenShift, err := r.runningOnOpenShift(hLog)
	if err != nil {
		return err
	}
	if !isOpenShift || is311 {
		hLog.Debug("non-OpenShift 4.x cluster detected, modifying hiveadmission webhooks for CA certs")
		err = r.injectCerts(apiService, validatingWebhooks, nil, hiveNSName, hLog)
		if err != nil {
			hLog.WithError(err).Error("error injecting certs")
			return err
		}
	}

	// Set the serving cert CA secret hash as an annotation on the pod template to force a rollout in the event it changes:
	servingCertSecret, err := r.hiveSecretLister.Secrets(hiveNSName).Get(hiveAdmissionServingCertSecretName)
	if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: hiveNSName, Name: hiveAdmissionServingCertSecretName}, servingCertSecret); err != nil {
		hLog.WithError(err).WithField("secretName", hiveAdmissionServingCertSecretName).Log(
			controllerutils.LogLevel(err), "error getting serving cert secret")
	}
	hLog.Info("Hashing serving cert secret onto a hiveadmission deployment annotation")
	certSecretHash := computeSecretDataHash(servingCertSecret.Data)
	if hiveAdmDeployment.Spec.Template.Annotations == nil {
		hiveAdmDeployment.Spec.Template.Annotations = map[string]string{}
	}
	hiveAdmDeployment.Spec.Template.Annotations[servingCertSecretHashAnnotation] = certSecretHash

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

func computeSecretDataHash(data map[string][]byte) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", data)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *ReconcileHiveConfig) getCACerts(hLog log.FieldLogger, hiveNSName string) ([]byte, []byte, error) {
	// Locate the kube CA by looking up secrets in hive namespace, finding one of
	// type 'kubernetes.io/service-account-token', and reading the CA off it.
	hLog.Debugf("listing secrets in %s namespace", hiveNSName)
	secrets := &corev1.SecretList{}
	err := r.Client.List(context.Background(), secrets, client.InNamespace(hiveNSName))
	if err != nil {
		hLog.WithError(err).Error("error listing secrets in hive namespace")
		return nil, nil, err
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

func (r *ReconcileHiveConfig) deployFeatureGatesConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig) (string, error) {
	cm := &corev1.ConfigMap{}
	cm.Name = featureGateConfigMapName
	cm.Namespace = getHiveNamespace(instance)
	cm.Data = make(map[string]string)

	cm.Data[constants.HiveFeatureGatesEnabledEnvVar] = ""
	if fg := instance.Spec.FeatureGates; fg != nil {
		s, ok := hivev1.FeatureSets[fg.FeatureSet]
		if ok && s != nil {
			cm.Data[constants.HiveFeatureGatesEnabledEnvVar] = strings.Join(s.Enabled, ",")
		}
		if fg.FeatureSet == hivev1.CustomFeatureSet && fg.Custom != nil {
			cm.Data[constants.HiveFeatureGatesEnabledEnvVar] = strings.Join(fg.Custom.Enabled, ",")
		}
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, cm, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying hive-feature-gates configmap")
		return "", err
	}
	hLog.WithField("result", result).Info("hive-feature-gates configmap applied")

	return computeConfigHash(cm), nil
}

// allowedContracts is the list of operator whitelisted contracts that hive will accept
// from CRDs.
var allowedContracts = sets.NewString(
	hivecontractsv1alpha1.ClusterInstallContractLabelKey,
)

func (r *ReconcileHiveConfig) deploySupportedContractsConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig) (string, error) {
	cm := &corev1.ConfigMap{}
	cm.Name = supportedContractsConfigMapName
	cm.Namespace = getHiveNamespace(instance)
	cm.Data = make(map[string]string)

	crdList := &apiextv1beta1.CustomResourceDefinitionList{}
	if err := r.Client.List(context.TODO(), crdList); err != nil {
		hLog.WithError(err).Error("error getting crds for collect contract implementations")
		return "", err
	}
	supported := map[string][]contracts.ContractImplementation{}
	for _, crd := range crdList.Items {
		// collect all the possible implementations from this crd
		var impls []contracts.ContractImplementation
		for _, version := range crd.Spec.Versions {
			impls = append(impls, contracts.ContractImplementation{
				Group:   crd.Spec.Group,
				Version: version.Name,
				Kind:    crd.Spec.Names.Kind,
			})
		}

		for label, val := range crd.ObjectMeta.Labels {
			if strings.HasPrefix(label, "contracts.hive.openshift.io") &&
				val == "true" &&
				allowedContracts.Has(label) { // lets store impls for this contract
				contract := strings.TrimPrefix(label, "contracts.hive.openshift.io/")
				curr := supported[contract]
				curr = append(curr, impls...)
				supported[contract] = curr
			}
		}
	}

	if len(supported) > 0 {
		var keys []string
		for k := range supported {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var config contracts.SupportedContractImplementationsList
		for _, c := range keys {
			config = append(config, contracts.SupportedContractImplementations{
				Name:      c,
				Supported: supported[c],
			})
		}

		configRaw, err := json.Marshal(config)
		if err != nil {
			hLog.WithError(err).Error("failed to marshal the supported contracts config")
			return "", err
		}
		cm.Data[supportedContractsConfigMapNameKey] = string(configRaw)
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, cm, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying hive-supported-contracts configmap")
		return "", err
	}
	hLog.WithField("result", result).Info("hive-supported-contracts configmap applied")

	return computeConfigHash(cm), nil
}

func addSupportedContractsConfigVolume(podSpec *corev1.PodSpec) {
	optional := true
	volume := corev1.Volume{}
	volume.Name = supportedContractsConfigMapName
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: supportedContractsConfigMapName,
		},
		Optional: &optional,
	}
	volumeMount := corev1.VolumeMount{
		Name:      supportedContractsConfigMapName,
		MountPath: supportedContractsConfigMapMountPath,
	}
	envVar := corev1.EnvVar{
		Name:  constants.SupportedContractImplementationsFileEnvVar,
		Value: fmt.Sprintf("%s/%s", supportedContractsConfigMapMountPath, supportedContractsConfigMapNameKey),
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVar)
}

func computeConfigHash(cm *corev1.ConfigMap) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", cm.Data)))
	return hex.EncodeToString(hasher.Sum(nil))
}
