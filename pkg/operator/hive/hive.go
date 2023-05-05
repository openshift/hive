package hive

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	oappsv1 "github.com/openshift/api/apps/v1"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
)

const (
	dnsServersEnvVar = "ZONE_CHECK_DNS_SERVERS"

	// hiveAdditionalCASecret is the name of the secret in the hive namespace
	// that will contain the aggregate of all AdditionalCertificateAuthorities
	// secrets specified in HiveConfig
	hiveAdditionalCASecret = "hive-additional-ca"

	// hiveConfigHashAnnotation is annotation on hivedeployment that contains
	// the hash of the contents of the hive-controllers-config configmap
	hiveConfigHashAnnotation = "hive.openshift.io/hiveconfig-hash"

	hiveClusterSyncStatefulSetSpecHashAnnotation = "hive.openshift.io/clustersync-statefulset-spec-hash"
)

var (
	controllersUsingReplicas = hivev1.ControllerNames{hivev1.ClustersyncControllerName}
)

func (r *ReconcileHiveConfig) deployHive(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, namespacesToClean []string, configHashes ...string) error {
	deploymentAsset := "config/controllers/deployment.yaml"
	namespacedAssets := []string{
		"config/controllers/service.yaml",
		"config/configmaps/install-log-regexes-configmap.yaml",
		"config/rbac/hive_frontend_serviceaccount.yaml",
		"config/controllers/hive_controllers_serviceaccount.yaml",
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
		// The hive-controller binary creates a configmap and lease to handle leader election. Delete them.
		// TODO: Dedup this with const cmd/manager/main.go:leaderElectionLockName
		lockName := "hive-controllers-leader"
		// TODO: Something better than hardcoding apiVersion and kind.
		toDel := map[string]string{
			"ConfigMap": "v1",
			"Lease":     "coordination.k8s.io/v1",
		}
		for kind, apiVersion := range toDel {
			hLog.Infof("Deleting %s/%s from old target namespace %s", kind, lockName, ns)
			// h.Delete already no-ops for IsNotFound
			if err := h.Delete(apiVersion, kind, ns, lockName); err != nil {
				return errors.Wrapf(err, "error deleting %s/%s from old target namespace %s", kind, lockName, ns)
			}
		}

	}

	asset := assets.MustAsset(deploymentAsset)
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveContainer, err := containerByName(&hiveDeployment.Spec.Template.Spec, "manager")
	if err != nil {
		return err
	}
	applyDeploymentConfig(instance, hivev1.DeploymentNameControllers, hiveContainer, hLog)

	hLog.Infof("hive image: %s", r.hiveImage)
	if r.hiveImage != "" {
		hiveContainer.Image = r.hiveImage
		hiveImageEnvVar := corev1.EnvVar{
			Name:  images.HiveImageEnvVar,
			Value: r.hiveImage,
		}

		hiveContainer.Env = append(hiveContainer.Env, hiveImageEnvVar)
	}

	if r.hiveImagePullPolicy != "" {
		hiveContainer.ImagePullPolicy = r.hiveImagePullPolicy

		hiveContainer.Env = append(
			hiveContainer.Env,
			corev1.EnvVar{
				Name:  images.HiveImagePullPolicyEnvVar,
				Value: string(r.hiveImagePullPolicy),
			},
		)
	}

	// Always add clustersync to the list of disabled controllers since clustersync is running in a statefulset now.
	disabledControllers := append(instance.Spec.DisabledControllers, "clustersync")
	hiveContainer.Args = append(hiveContainer.Args, "--disabled-controllers", strings.Join(disabledControllers, ","))

	if level := instance.Spec.LogLevel; level != "" {
		hiveContainer.Args = append(hiveContainer.Args, "--log-level", level)
	}

	if syncSetReapplyInterval := instance.Spec.SyncSetReapplyInterval; syncSetReapplyInterval != "" {
		syncsetReapplyIntervalEnvVar := corev1.EnvVar{
			Name:  "SYNCSET_REAPPLY_INTERVAL",
			Value: syncSetReapplyInterval,
		}

		hiveContainer.Env = append(hiveContainer.Env, syncsetReapplyIntervalEnvVar)
	}

	addConfigVolume(&hiveDeployment.Spec.Template.Spec, managedDomainsConfigMapInfo, hiveContainer)
	addConfigVolume(&hiveDeployment.Spec.Template.Spec, awsPrivateLinkConfigMapInfo, hiveContainer)
	addConfigVolume(&hiveDeployment.Spec.Template.Spec, failedProvisionConfigMapInfo, hiveContainer)
	addConfigVolume(&hiveDeployment.Spec.Template.Spec, metricsConfigConfigMapInfo, hiveContainer)

	// This triggers the clusterdeployment controller to copy the secret into the CD's namespace.
	// It would be neat if it did that purely based on the FailedProvisionConfig ConfigMap, to
	// which it does have access, but that code path is shared by other things that need the
	// same copied secret.
	if awsSpec := instance.Spec.FailedProvisionConfig.AWS; awsSpec != nil {
		hiveContainer.Env = append(hiveContainer.Env, corev1.EnvVar{
			Name:  constants.InstallLogsCredentialsSecretRefEnvVar,
			Value: awsSpec.CredentialsSecretRef.Name,
		})
	}

	hiveNSName := getHiveNamespace(instance)

	if awssp := instance.Spec.ServiceProviderCredentialsConfig.AWS; awssp != nil && awssp.CredentialsSecretRef.Name != "" {
		hiveContainer.Env = append(hiveContainer.Env, corev1.EnvVar{
			Name:  constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar,
			Value: awssp.CredentialsSecretRef.Name,
		})
	}

	if zoneCheckDNSServers := os.Getenv(dnsServersEnvVar); len(zoneCheckDNSServers) > 0 {
		dnsServersEnvVar := corev1.EnvVar{
			Name:  dnsServersEnvVar,
			Value: zoneCheckDNSServers,
		}
		hiveContainer.Env = append(hiveContainer.Env, dnsServersEnvVar)
	}

	if instance.Spec.Backup.Velero.Enabled {
		hLog.Infof("Velero Backup Enabled.")
		tmpEnvVar := corev1.EnvVar{
			Name:  constants.VeleroBackupEnvVar,
			Value: "true",
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)

		if instance.Spec.Backup.Velero.Namespace != "" {
			hLog.Infof("Velero Backup Namespace specified.")
			tmpEnvVar := corev1.EnvVar{
				Name:  constants.VeleroNamespaceEnvVar,
				Value: instance.Spec.Backup.Velero.Namespace,
			}
			hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
		}
	}

	if instance.Spec.ArgoCD.Enabled {
		hLog.Infof("ArgoCD integration enabled")
		tmpEnvVar := corev1.EnvVar{
			Name:  constants.ArgoCDEnvVar,
			Value: "true",
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.ArgoCD.Namespace != "" {
		hLog.Infof("ArgoCD namespace specified in hiveconfig")
		tmpEnvVar := corev1.EnvVar{
			Name:  constants.ArgoCDNamespaceEnvVar,
			Value: instance.Spec.ArgoCD.Namespace,
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.DeprovisionsDisabled != nil && *instance.Spec.DeprovisionsDisabled {
		hLog.Info("deprovisions disabled in hiveconfig")
		tmpEnvVar := corev1.EnvVar{
			Name:  constants.DeprovisionsDisabledEnvVar,
			Value: "true",
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.Backup.MinBackupPeriodSeconds != nil {
		hLog.Infof("MinBackupPeriodSeconds specified.")
		tmpEnvVar := corev1.EnvVar{
			Name:  constants.MinBackupPeriodSecondsEnvVar,
			Value: strconv.Itoa(*instance.Spec.Backup.MinBackupPeriodSeconds),
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.DeleteProtection == hivev1.DeleteProtectionEnabled {
		hLog.Info("Delete Protection enabled")
		hiveContainer.Env = append(hiveContainer.Env, corev1.EnvVar{
			Name:  constants.ProtectedDeleteEnvVar,
			Value: "true",
		})
	}

	if instance.Spec.ReleaseImageVerificationConfigMapRef != nil {
		hLog.Info("Release Image verification enabled")
		hiveContainer.Env = append(hiveContainer.Env, corev1.EnvVar{
			Name:  constants.HiveReleaseImageVerificationConfigMapNamespaceEnvVar,
			Value: instance.Spec.ReleaseImageVerificationConfigMapRef.Namespace,
		}, corev1.EnvVar{
			Name:  constants.HiveReleaseImageVerificationConfigMapNameEnvVar,
			Value: instance.Spec.ReleaseImageVerificationConfigMapRef.Name,
		})
	}

	if err := r.includeAdditionalCAs(hLog, h, instance, hiveDeployment, hiveContainer, namespacesToClean); err != nil {
		return err
	}

	r.includeGlobalPullSecret(hLog, h, instance, hiveContainer)

	if instance.Spec.MaintenanceMode != nil && *instance.Spec.MaintenanceMode {
		hLog.Warn("maintenanceMode enabled in HiveConfig, setting hive-controllers replicas to 0")
		replicas := int32(0)
		hiveDeployment.Spec.Replicas = &replicas
	}

	if hiveDeployment.Spec.Template.Annotations == nil {
		hiveDeployment.Spec.Template.Annotations = make(map[string]string, 1)
	}

	httpProxy, httpsProxy, noProxy, err := r.discoverProxyVars()
	if err != nil {
		return err
	}
	utils.SetProxyEnvVars(&hiveDeployment.Spec.Template.Spec, httpProxy, httpsProxy, noProxy)

	// Include the proxy vars in the hash so we redeploy if they change
	hiveDeployment.Spec.Template.Annotations[hiveConfigHashAnnotation] = computeHash(
		httpProxy+httpsProxy+noProxy, configHashes...)

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
		"config/rbac/hive_frontend_role.yaml",
		"config/controllers/hive_controllers_role.yaml",
	}
	for _, a := range applyAssets {
		if err := util.ApplyAssetWithGC(h, a, instance, hLog); err != nil {
			hLog.WithField("asset", a).WithError(err).Error("error applying asset")
			return err
		}
	}

	// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
	clusterRoleBindingAssets := []string{
		"config/rbac/hive_frontend_role_binding.yaml",
		"config/controllers/hive_controllers_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {

		if err := util.ApplyClusterRoleBindingAssetWithSubjectNSOverrideAndGC(h, crbAsset, hiveNSName, instance); err != nil {
			hLog.WithError(err).Error("error applying ClusterRoleBinding with namespace override")
			return err
		}
		hLog.WithField("asset", crbAsset).Info("applied ClusterRoleRoleBinding asset with namespace override")
	}

	// In very rare cases we use OpenShift specific types which will not apply if running on
	// vanilla Kubernetes. Detect this and skip if so.
	// NOTE: We are configuring two role bindings here, but they do not use namespaced ServiceAccount subjects.
	// (rather global OpenShift groups), thus they do not need namespace override behavior.
	openshiftSpecificAssets := []string{
		"config/rbac/hive_admin_role.yaml",
		"config/rbac/hive_reader_role.yaml",
		"config/rbac/hive_clusterpool_admin.yaml",
		"config/rbac/hive_admin_role_binding.yaml",
		"config/rbac/hive_reader_role_binding.yaml",
	}
	isOpenShift, err := r.runningOnOpenShift(hLog)
	if err != nil {
		return err
	}
	if isOpenShift {
		hLog.Info("deploying OpenShift specific assets")
		for _, a := range openshiftSpecificAssets {
			err = util.ApplyAssetWithGC(h, a, instance, hLog)
			if err != nil {
				return err
			}
		}
	} else {
		hLog.Warn("hive is not running on OpenShift, some optional assets will not be deployed")
	}

	// Apply nodeSelector and tolerations passed through from the operator deployment
	hiveDeployment.Spec.Template.Spec.NodeSelector = r.nodeSelector
	hiveDeployment.Spec.Template.Spec.Tolerations = r.tolerations

	hiveDeployment.Namespace = hiveNSName
	result, err := util.ApplyRuntimeObjectWithGC(h, hiveDeployment, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("hive-controllers deployment applied (%s)", result)

	hLog.Info("all hive components successfully reconciled")
	return nil
}

func (r *ReconcileHiveConfig) includeAdditionalCAs(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, hiveDeployment *appsv1.Deployment, hiveContainer *corev1.Container, namespacesToClean []string) error {
	// Delete any additional CA secrets from previous target namespaces
	for _, ns := range namespacesToClean {
		hLog.Infof("Deleting secret/%s from old target namespace %s", hiveAdditionalCASecret, ns)
		// h.Delete already no-ops for IsNotFound
		// TODO: Something better than hardcoding apiVersion and kind.
		if err := h.Delete("v1", "Secret", ns, hiveAdditionalCASecret); err != nil {
			return errors.Wrapf(err, "error deleting secret/%s from old target namespace %s", hiveAdditionalCASecret, ns)
		}
	}

	hiveNS := getHiveNamespace(instance)
	additionalCA := &bytes.Buffer{}
	for _, clientCARef := range instance.Spec.AdditionalCertificateAuthoritiesSecretRef {
		caSecret, err := r.hiveSecretLister.Secrets(hiveNS).Get(clientCARef.Name)
		if err != nil {
			hLog.WithError(err).WithField("secret", clientCARef.Name).Errorf("Cannot read client CA secret")
			continue
		}
		crt, ok := caSecret.Data["ca.crt"]
		if !ok {
			hLog.WithField("secret", clientCARef.Name).Warning("Secret does not contain expected key (ca.crt)")
		}
		fmt.Fprintf(additionalCA, "%s\n", crt)
	}

	if additionalCA.Len() == 0 {
		caSecret, err := r.hiveSecretLister.Secrets(hiveNS).Get(hiveAdditionalCASecret)
		if err == nil {
			err = h.Delete(caSecret.APIVersion, caSecret.Kind, caSecret.Namespace, caSecret.Name)
			if err != nil {
				hLog.WithError(err).WithField("secret", fmt.Sprintf("%s/%s", hiveNS, hiveAdditionalCASecret)).
					Error("cannot delete hive additional ca secret")
				return err
			}
		}
		return nil
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: getHiveNamespace(instance),
			Name:      hiveAdditionalCASecret,
		},
		Data: map[string][]byte{
			"ca.crt": additionalCA.Bytes(),
		},
	}
	result, err := util.ApplyRuntimeObjectWithGC(h, caSecret, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying additional cert secret")
		return err
	}
	hLog.Infof("additional cert secret applied (%s)", result)

	// Generating a volume name with a hash based on the contents of the additional CA
	// secret will ensure that when there are changes to the secret, the hive controller
	// will be re-deployed.
	hash := fmt.Sprintf("%x", md5.Sum(additionalCA.Bytes()))
	volumeName := fmt.Sprintf("additionalca-%s", hash[:20])

	hiveDeployment.Spec.Template.Spec.Volumes = append(hiveDeployment.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: hiveAdditionalCASecret,
			},
		},
	})

	hiveContainer.VolumeMounts = append(hiveContainer.VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/additional/ca",
		ReadOnly:  true,
	})

	hiveContainer.Env = append(hiveContainer.Env, corev1.EnvVar{
		Name:  "ADDITIONAL_CA",
		Value: "/additional/ca/ca.crt",
	})

	return nil
}

func (r *ReconcileHiveConfig) includeGlobalPullSecret(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, hiveContainer *corev1.Container) {
	if instance.Spec.GlobalPullSecretRef == nil || instance.Spec.GlobalPullSecretRef.Name == "" {
		hLog.Debug("GlobalPullSecret is not provided in HiveConfig, it will not be deployed")
		return
	}

	globalPullSecretEnvVar := corev1.EnvVar{
		Name:  constants.GlobalPullSecret,
		Value: instance.Spec.GlobalPullSecretRef.Name,
	}
	hiveContainer.Env = append(hiveContainer.Env, globalPullSecretEnvVar)
}

func (r *ReconcileHiveConfig) runningOnOpenShift(hLog log.FieldLogger) (bool, error) {
	deploymentConfigGroupVersion := oappsv1.GroupVersion.String()
	list, err := r.discoveryClient.ServerResourcesForGroupVersion(deploymentConfigGroupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			hLog.WithError(err).Debug("DeploymentConfig objects not found, not running on OpenShift")
			return false, nil
		}
		hLog.WithError(err).Error("Error determining whether running on OpenShift")
		return false, err
	}

	return len(list.APIResources) > 0, nil
}

func (r *ReconcileHiveConfig) cleanupLegacySyncSetInstances(hLog log.FieldLogger) error {
	crdClient := r.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1beta1",
		Resource: "customresourcedefinitions",
	})
	syncSetInstanceCRD, err := crdClient.Get(context.Background(), "syncsetinstances.hive.openshift.io", metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		hLog.Debug("syncsetinstance crd has already been deleted")
		return nil
	case err != nil:
		return errors.Wrap(err, "could not get the syncsetinstance CRD")
	}
	// Delete all the SyncSetInstance. List until there are no more SyncSetInstances to catch SyncSetInstances that may
	// have been created since the last List was run. There should not be more SyncSetInstance created since the Hive
	// controller that would create SyncSetInstances should not be running, but let's List until there is a zero count
	// just in case.
	for {
		numberDeleted, err := r.deleteAllSyncSetInstances(hLog)
		if err != nil {
			return err
		}
		if numberDeleted == 0 {
			break
		}
	}
	hLog.Info("Deleting SyncSetInstance CRD")
	if err := crdClient.Delete(context.Background(), syncSetInstanceCRD.GetName(), metav1.DeleteOptions{}); err != nil {
		return errors.Wrap(err, "failed to delete syncsetinstance CRD")
	}
	return nil
}

func (r *ReconcileHiveConfig) deleteAllSyncSetInstances(hLog log.FieldLogger) (numberDeleted int, returnErr error) {
	syncSetInstanceClient := r.dynamicClient.Resource(hivev1.SchemeGroupVersion.WithResource("syncsetinstances"))
	hLog.Info("deleting SyncSetInstances")
	listOptions := metav1.ListOptions{}
	for {
		syncSetInstanceList, err := syncSetInstanceClient.List(context.Background(), listOptions)
		if err != nil {
			return numberDeleted, errors.Wrap(err, "failed to list SyncSetInstances")
		}
		hLog.WithField("numberDeleted", numberDeleted).WithField("batchSize", len(syncSetInstanceList.Items)).Infof("deleting the next batch of SyncSetInstances")
		var errs []error
		for _, syncSetInstance := range syncSetInstanceList.Items {
			c := syncSetInstanceClient.Namespace(syncSetInstance.GetNamespace())
			resourceVersion := syncSetInstance.GetResourceVersion()
			if len(syncSetInstance.GetFinalizers()) != 0 {
				syncSetInstance.SetFinalizers(nil)
				updatedSyncSetInstance, err := c.Update(context.Background(), &syncSetInstance, metav1.UpdateOptions{})
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "failed to remove finalizers from SyncSetInstance %s/%s", syncSetInstance.GetNamespace(), syncSetInstance.GetName()))
					continue
				}
				resourceVersion = updatedSyncSetInstance.GetResourceVersion()
			}
			// Ensure that we are deleting the SyncSetInstance version to which we just updated. In case the Hive
			// syncsetinstance controller is still running, this will protect against the controller putting back the
			// finalizer between when we removed the finalizers and when we did the delete. If the controller is running
			// and puts back the finalizer, then the controller may attempt to delete synced resources in the target
			// cluster.
			uid := syncSetInstance.GetUID()
			switch err := c.Delete(
				context.Background(),
				syncSetInstance.GetName(),
				metav1.DeleteOptions{
					Preconditions: &metav1.Preconditions{
						UID:             &uid,
						ResourceVersion: &resourceVersion,
					},
				},
			); {
			case err == nil, apierrors.IsNotFound(err):
				numberDeleted++
			default:
				errs = append(errs, errors.Wrapf(err, "failed to delete SyncSetInstance %s/%s", syncSetInstance.GetNamespace(), syncSetInstance.GetName()))
			}
		}
		if len(errs) != 0 {
			return numberDeleted, utilerrors.NewAggregate(errs)
		}
		cont := syncSetInstanceList.GetContinue()
		if cont == "" {
			break
		}
		listOptions.Continue = cont
	}
	return
}

func (r *ReconcileHiveConfig) discoverProxyVars() (string, string, string, error) {
	httpProxy, httpsProxy, noProxy := os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("NO_PROXY")

	// We'll assume that if *any* of these are set, we don't need to read the cluster proxy object
	if httpProxy+httpsProxy+noProxy != "" {
		return httpProxy, httpsProxy, noProxy, nil
	}

	proxy := &configv1.Proxy{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, proxy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("No cluster proxy found")
			return "", "", "", nil
		}
		return "", "", "", errors.Wrap(err, "Failed to load cluster proxy object")
	}
	return proxy.Status.HTTPProxy, proxy.Status.HTTPSProxy, proxy.Status.NoProxy, nil
}
