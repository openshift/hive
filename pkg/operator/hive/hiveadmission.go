package hive

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	"github.com/openshift/hive/pkg/client/clientset/versioned/scheme"
	hiveconstants "github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/contracts"

	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
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

func (r *ReconcileHiveConfig) deployHiveAdmission(hLog log.FieldLogger, cpHelper, dpHelper resource.Helper, instance *hivev1.HiveConfig, hiveNSName string, namespacesToClean []string, additionalHashes ...string) error {
	deploymentAsset := "config/hiveadmission/deployment.yaml"
	cpNamespacedAssets := []string{
		"config/hiveadmission/service.yaml",
		"config/hiveadmission/service-account.yaml",
	}
	dpNamespacedAssets := []string{
		// Modified before applying. For cleanup, the kind+namespace+name matches,
		// which is sufficient.
		"config/hiveadmission/service.yaml",
		"config/hiveadmission/konnectivity-agent.yaml",
		"config/hiveadmission/endpoints.yaml",
	}
	dpGlobalAssets := []string{
		// Data plane global assets are still specific to one data plane.
		"config/hiveadmission/apiservice.yaml",
	}

	////////////////
	// CLEAN UP DELETED FORMER TARGET NAMESPACES / DATA PLANES
	////////////////
	assetsToClean := append(cpNamespacedAssets, deploymentAsset)
	for _, ns := range namespacesToClean {
		for _, asset := range assetsToClean {
			hLog.
				WithField("plane", "control").
				WithField("asset", asset).
				WithField("oldTargetNamespace", ns).
				Info("Deleting asset from old target namespace")
			// DeleteAssetWithNSOverride already no-ops for IsNotFound
			if err := util.DeleteAssetWithNSOverride(cpHelper, asset, ns, instance); err != nil {
				return errors.Wrapf(err, "error deleting asset %s from old target namespace %s", asset, ns)
			}
		}
	}
	if isScaleMode(instance) {
		for _, ns := range namespacesToClean {
			for _, asset := range dpNamespacedAssets {
				hLog.
					WithField("plane", "data").
					WithField("asset", asset).
					WithField("oldTargetNamespace", ns).
					Info("Deleting asset from old target namespace")
				// DeleteAssetWithNSOverride already no-ops for IsNotFound
				if err := util.DeleteAssetWithNSOverride(dpHelper, asset, ns, instance); err != nil {
					return errors.Wrapf(err, "error deleting asset %s from old target namespace %s on data plane", asset, ns)
				}
			}
		}
		// Scrub the non-namespaced assets from the data plane.
		assetsToClean = append(dpGlobalAssets, webhookAssets...)
		for _, asset := range assetsToClean {
			hLog.
				WithField("plane", "data").
				WithField("asset", asset).
				Info("Deleting global asset")
			// DeleteAssetWithNSOverride already no-ops for IsNotFound.
			// An empty namespace arg works for global assets.
			if err := util.DeleteAssetWithNSOverride(dpHelper, asset, "", instance); err != nil {
				return errors.Wrapf(err, "error deleting global asset %s from data plane", asset)
			}

		}
	}

	////////////////
	// APPLY UNMODIFIED NAMESPACED ASSETS TO THE CONTROL PLANE
	////////////////

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	for _, assetPath := range cpNamespacedAssets {
		if err := util.ApplyAssetWithNSOverrideAndGC(cpHelper, assetPath, hiveNSName, instance); err != nil {
			hLog.WithError(err).Error("error applying object with namespace override")
			return err
		}
		hLog.WithField("asset", assetPath).Info("applied asset with namespace override")
	}

	////////////////
	// APPLY HEADLESS SERVICE AND PROXY IN SCALE MODE
	// To make admission work in scale mode, we need to create a network connection
	// *from* the data plane *to* the hiveadmission service in the control plane.
	// NOTE: This is currently hypershift-specific! For a generic data plane, I
	// think we would (at least) put the konnectivity-agent in the data plane rather
	// than the control plane.
	////////////////
	if isScaleMode(instance) {
		// Grab the real Service from the control plane. We need its IP for
		// - the konnectivity-agent
		// - the Endpoints object in the data plane
		svc := &corev1.Service{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: "hiveadmission", Namespace: hiveNSName}, svc); err != nil {
			hLog.WithError(err).Error("error retrieving hiveadmission Service in the control plane")
			return err
		}
		svcIP := svc.Spec.ClusterIP
		if svcIP == "" {
			// Should we poll this to give it some time to be generated? Meh, it ought to appear by the
			// next reconcile; and the code that creates the Service *should* be idempotent, so its IP
			// shouldn't change.
			return errors.New("hiveadmission Service has no ClusterIP assigned")
		}

		// Deploy a headless Service to the data plane
		// https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors
		dummySvc := util.ReadServiceV1OrDie(assets.MustAsset("config/hiveadmission/service.yaml"))
		dummySvc.Namespace = hiveNSName
		// This prevents k8s from automatically syncing the Endpoints (removing its Subsets, making it useless)
		dummySvc.Spec.Selector = nil
		dummySvc.Spec.Ports[0].TargetPort = intstr.IntOrString{}
		if _, err := dpHelper.ApplyRuntimeObject(dummySvc, scheme.Scheme); err != nil {
			return errors.Wrap(err, "failed to create dummy Service in data plane")
		}
		hLog.Infof("headless Service applied to data plane")

		// Create the Endpoints in the data plane with the same namespace/name as the data plane Service,
		// but the IP points to the control plane Service.
		// TODO: Convert to EndpointSlice, as Endpoints is "legacy":
		//       https://kubernetes.io/docs/concepts/services-networking/service/#endpoints
		ep := util.ReadEndpointsV1OrDie(assets.MustAsset("config/hiveadmission/endpoints.yaml"))
		for ssi := range ep.Subsets {
			ep.Subsets[ssi].Addresses = []corev1.EndpointAddress{{IP: svcIP}}
		}
		ep.Namespace = hiveNSName
		if _, err := dpHelper.ApplyRuntimeObject(ep, scheme.Scheme); err != nil {
			return errors.Wrap(err, "failed to create hiveadmission Endpoints")
		}
		hLog.Info("hiveadmission Endpoints applied in data plane")

		// Grab the konnectivity-server deployment from the control plane. We'll use the same image
		// URI for our agent.
		ks := &appsv1.Deployment{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: "konnectivity-server", Namespace: hiveNSName}, ks); err != nil {
			hLog.WithError(err).Error("error retrieving konnectivity-server Deployment in the contol plane -- currently scale mode only works with hypershift!")
		}

		// Deploy the konnectivity agent into the control plane with the IP of the control plane Service
		kad := util.ReadDeploymentV1OrDie(assets.MustAsset("config/hiveadmission/konnectivity-agent.yaml"))
		kad.Namespace = hiveNSName
		kad.Spec.Template.Spec.Containers[0].Image = ks.Spec.Template.Spec.Containers[0].Image
		// This is kind of ugh...
		for i, arg := range kad.Spec.Template.Spec.Containers[0].Args {
			if strings.HasPrefix(arg, "ipv4=") {
				kad.Spec.Template.Spec.Containers[0].Args[i] = "ipv4=" + svcIP
			}
		}
		if _, err := cpHelper.ApplyRuntimeObject(kad, scheme.Scheme); err != nil {
			return errors.Wrap(err, "failed to deploy konnectivity-agent")
		}

	}

	////////////////
	// APPLY WEBHOOK CONFIGURATIONS AND APISERVICE (GLOBAL)
	// NOTE: We do this here (within the namespace-scoped function) because in
	// scale mode, we really are applying the configs multiple times -- once to
	// each data plane. And in non-scale mode, we're only hitting this function
	// once anyway.
	////////////////

	for _, asset := range webhookAssets {
		if isScaleMode(instance) {
			// In scale mode, webhook configs are cleaned up explicitly from the data plane.
			if err := util.ApplyAsset(dpHelper, asset, hLog.WithField("plane", "data")); err != nil {
				hLog.WithField("webhookAsset", asset).WithError(err).Errorf("error applying validating webhook")
				return err
			}
		} else {
			// In non-scale mode, we can set OwnerReferences on these to point to the
			// HiveConfig, so they get cleaned up if it's deleted.
			wh := util.ReadValidatingWebhookConfigurationV1OrDie(assets.MustAsset(asset))
			result, err := util.ApplyRuntimeObjectWithGC(cpHelper, wh, instance)
			if err != nil {
				hLog.WithField("webhook", wh.Name).WithError(err).Errorf("error applying validating webhook")
				return err
			}
			hLog.WithField("webhook", wh.Name).WithField("result", result).Info("applied validating webhook")
		}
	}

	hLog.Debug("reading apiservice")
	asset := assets.MustAsset("config/hiveadmission/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset)
	apiService.Spec.Service.Namespace = hiveNSName

	// If we're running on vanilla Kube (mostly devs using kind), or scale mode, we
	// will not have access to the service cert injection we normally use. Lookup
	// the cluster CA and inject into the APIService.
	// NOTE: If this is vanilla kube, you will also need to manually create a certificate
	// secret, see hack/hiveadmission-dev-cert.sh. (TODO: automate -- see HIVE-1449.)
	if isScaleMode(instance) || !r.isOpenShift {
		hLog.Debug("scale mode or non-OpenShift cluster detected, modifying APIService for CA certs")
		svcCA, err := r.getServiceCACert(hLog, hiveNSName)
		if err != nil {
			return err
		}
		apiService.Spec.CABundle = svcCA
	}

	if isScaleMode(instance) {
		// In scale mode, the APIService is cleaned up explicitly from the data plane.
		result, err := dpHelper.ApplyRuntimeObject(apiService, scheme.Scheme)
		if err != nil {
			hLog.WithError(err).Errorf("error applying APIService")
		}
		hLog.WithField("plane", "data").WithField("result", result).Info("applied APIService")
	} else {

		result, err := util.ApplyRuntimeObjectWithGC(cpHelper, apiService, instance)
		if err != nil {
			hLog.WithError(err).Error("error applying apiservice")
			return err
		}
		hLog.WithField("plane", "control").WithField("result", result).Info("applied APIService")
	}

	////////////////
	// LOAD, MODIFY, AND APPLY THE HIVEADMISSION DEPLOYMENT
	////////////////

	asset = assets.MustAsset(deploymentAsset)
	hLog.Debug("reading deployment")
	hiveAdmDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveAdmContainer, err := containerByName(&hiveAdmDeployment.Spec.Template.Spec, "hiveadmission")
	if err != nil {
		return err
	}

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

	hiveAdmDeployment.Spec.Template.ObjectMeta.Annotations[inputHashAnnotation] = computeHash("", additionalHashes...)
	controllerutils.SetProxyEnvVars(&hiveAdmDeployment.Spec.Template.Spec,
		os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("NO_PROXY"))

	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, managedDomainsConfigMapInfo, hiveAdmContainer)
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, awsPrivateLinkConfigMapInfo, hiveAdmContainer)
	addConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, r.supportedContractsConfigMapInfo(), hiveAdmContainer)
	addReleaseImageVerificationConfigMapEnv(hiveAdmContainer, instance)

	if isScaleMode(instance) {
		// In scale mode, the admission service framework needs to be able to
		// authenticate to the data plane (TODO: Why??).
		controllerutils.AddDataPlaneKubeConfigVolume(&hiveAdmDeployment.Spec.Template.Spec, hiveAdmContainer, true)
	}

	// Set the serving cert CA secret hash as an annotation on the pod template to force a rollout in the event it changes:
	servingCertSecret, err := r.hiveSecretListers[hiveNSName].Get(hiveAdmissionServingCertSecretName)
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

	result, err := util.ApplyRuntimeObjectWithGC(cpHelper, hiveAdmDeployment, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.WithField("result", result).Info("hiveadmission deployment applied")

	hLog.Info("hiveadmission components reconciled successfully")
	return nil
}

func (r *ReconcileHiveConfig) getServiceCACert(hLog log.FieldLogger, hiveNSName string) ([]byte, error) {
	// Locate the kube CA by looking up secrets in hive namespace, finding one of
	// type 'kubernetes.io/service-account-token', and reading the CA off it.
	hLog.Debugf("listing secrets in %s namespace", hiveNSName)
	secrets, err := r.hiveSecretListers[hiveNSName].List(labels.Everything())
	if err != nil {
		hLog.WithError(err).Error("error listing secrets in hive namespace")
		return nil, err
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
		return nil, fmt.Errorf("no %s secrets found", corev1.SecretTypeServiceAccountToken)
	}
	kubeCA, ok := firstSATokenSecret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("secret %s did not contain key ca.crt", firstSATokenSecret.Name)
	}

	// Load the service CA:
	serviceCA, ok := firstSATokenSecret.Data["service-ca.crt"]
	if !ok {
		hLog.Warnf("secret %s did not contain key service-ca.crt, likely not running on OpenShift, using ca.crt instead", firstSATokenSecret.Name)
		serviceCA = kubeCA
	}

	return serviceCA, nil
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
