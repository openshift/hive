package hive

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hiveconstants "github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	oappsv1 "github.com/openshift/api/apps/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	dnsServersEnvVar = "ZONE_CHECK_DNS_SERVERS"

	// hiveAdditionalCASecret is the name of the secret in the hive namespace
	// that will contain the aggregate of all AdditionalCertificateAuthorities
	// secrets specified in HiveConfig
	hiveAdditionalCASecret = "hive-additional-ca"
)

func (r *ReconcileHiveConfig) deployHive(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder, mdConfigMap *corev1.ConfigMap) error {

	asset := assets.MustAsset("config/controllers/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveContainer := &hiveDeployment.Spec.Template.Spec.Containers[0]

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

	if level := instance.Spec.LogLevel; level != "" {
		hiveDeployment.Spec.Template.Spec.Containers[0].Command = append(
			hiveDeployment.Spec.Template.Spec.Containers[0].Command,
			"--log-level",
			level,
		)
	}

	if syncSetReapplyInterval := instance.Spec.SyncSetReapplyInterval; syncSetReapplyInterval != "" {
		syncsetReapplyIntervalEnvVar := corev1.EnvVar{
			Name:  "SYNCSET_REAPPLY_INTERVAL",
			Value: syncSetReapplyInterval,
		}

		hiveContainer.Env = append(hiveContainer.Env, syncsetReapplyIntervalEnvVar)
	}

	addManagedDomainsVolume(&hiveDeployment.Spec.Template.Spec, mdConfigMap.Name)

	hiveNSName := getHiveNamespace(instance)

	// By default we will try to gather logs on failed installs:
	logsEnvVar := corev1.EnvVar{
		Name:  constants.SkipGatherLogsEnvVar,
		Value: strconv.FormatBool(instance.Spec.FailedProvisionConfig.SkipGatherLogs),
	}
	hiveContainer.Env = append(hiveContainer.Env, logsEnvVar)

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
			Name:  hiveconstants.VeleroBackupEnvVar,
			Value: "true",
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.DeprovisionsDisabled != nil && *instance.Spec.DeprovisionsDisabled {
		hLog.Info("deprovisions disabled in hiveconfig")
		tmpEnvVar := corev1.EnvVar{
			Name:  hiveconstants.DeprovisionsDisabledEnvVar,
			Value: "true",
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if instance.Spec.Backup.MinBackupPeriodSeconds != nil {
		hLog.Infof("MinBackupPeriodSeconds specified.")
		tmpEnvVar := corev1.EnvVar{
			Name:  hiveconstants.MinBackupPeriodSecondsEnvVar,
			Value: strconv.Itoa(*instance.Spec.Backup.MinBackupPeriodSeconds),
		}
		hiveContainer.Env = append(hiveContainer.Env, tmpEnvVar)
	}

	if err := r.includeAdditionalCAs(hLog, h, instance, hiveDeployment); err != nil {
		return err
	}

	r.includeGlobalPullSecret(hLog, h, instance, hiveDeployment)

	if instance.Spec.MaintenanceMode != nil && *instance.Spec.MaintenanceMode {
		hLog.Warn("maintenanceMode enabled in HiveConfig, setting hive-controllers replicas to 0")
		replicas := int32(0)
		hiveDeployment.Spec.Replicas = &replicas
	}

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	namespacedAssets := []string{
		"config/controllers/service.yaml",
		"config/configmaps/install-log-regexes-configmap.yaml",
		"config/rbac/hive_frontend_serviceaccount.yaml",
		"config/controllers/hive_controllers_serviceaccount.yaml",
	}
	for _, assetPath := range namespacedAssets {
		if err := applyAssetWithNamespaceOverride(h, assetPath, hiveNSName); err != nil {
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
		if err := util.ApplyAsset(h, a, hLog); err != nil {
			return err
		}
	}

	// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
	clusterRoleBindingAssets := []string{
		"config/rbac/hive_frontend_role_binding.yaml",
		"config/controllers/hive_controllers_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {
		if err := applyClusterRoleBindingAssetWithSubjectNamespaceOverride(h, crbAsset, hiveNSName); err != nil {
			hLog.WithError(err).Error("error applying ClusterRoleBinding with namespace override")
			return err
		}
		hLog.WithField("asset", crbAsset).Info("applied ClusterRoleRoleBinding asset with namespace override")
	}

	// Due to bug with OLM not updating CRDs on upgrades, we are re-applying
	// the latest in the operator to ensure updates roll out.
	// TODO: Attempt removing this once Hive is running purely on 4.x,
	// as it requires a significant privilege escalation we would rather
	// leave in the hands of OLM.
	crdFiles, err := assets.AssetDir("config/crds")
	if err != nil {
		return errors.Wrap(err, "error listing CRD asset files")
	}
	for _, a := range crdFiles {
		crdPath := filepath.Join("config/crds", a)
		if err := util.ApplyAsset(h, crdPath, hLog); err != nil {
			return errors.Wrapf(err, "unable to apply CRD %s", crdPath)
		}
	}

	// In very rare cases we use OpenShift specific types which will not apply if running on
	// vanilla Kubernetes. Detect this and skip if so.
	// NOTE: We are configuring two role bindings here, but they do not use namespaced ServiceAccount subjects.
	// (rather global OpenShift groups), thus they do not need namespace override behavior.
	openshiftSpecificAssets := []string{
		"config/rbac/hive_admin_role.yaml",
		"config/rbac/hive_reader_role.yaml",
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
			err = util.ApplyAsset(h, a, hLog)
			if err != nil {
				return err
			}
		}
	} else {
		hLog.Warn("hive is not running on OpenShift, some optional assets will not be deployed")
	}

	hiveDeployment.Namespace = hiveNSName
	result, err := h.ApplyRuntimeObject(hiveDeployment, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("hive-controllers deployment applied (%s)", result)

	hLog.Info("all hive components successfully reconciled")
	return nil
}

func (r *ReconcileHiveConfig) includeAdditionalCAs(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, hiveDeployment *appsv1.Deployment) error {
	additionalCA := &bytes.Buffer{}
	for _, clientCARef := range instance.Spec.AdditionalCertificateAuthoritiesSecretRef {
		caSecret := &corev1.Secret{}
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: getHiveNamespace(instance), Name: clientCARef.Name}, caSecret)
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
		caSecret := &corev1.Secret{}
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: getHiveNamespace(instance), Name: hiveAdditionalCASecret}, caSecret)
		if err == nil {
			err = r.Delete(context.TODO(), caSecret)
			if err != nil {
				hLog.WithError(err).WithField("secret", fmt.Sprintf("%s/%s", getHiveNamespace(instance), hiveAdditionalCASecret)).
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
	result, err := h.ApplyRuntimeObject(caSecret, scheme.Scheme)
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

	hiveDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(hiveDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/additional/ca",
		ReadOnly:  true,
	})

	hiveDeployment.Spec.Template.Spec.Containers[0].Env = append(hiveDeployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "ADDITIONAL_CA",
		Value: "/additional/ca/ca.crt",
	})

	return nil
}

func (r *ReconcileHiveConfig) includeGlobalPullSecret(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, hiveDeployment *appsv1.Deployment) {
	if instance.Spec.GlobalPullSecretRef == nil || instance.Spec.GlobalPullSecretRef.Name == "" {
		hLog.Debug("GlobalPullSecret is not provided in HiveConfig, it will not be deployed")
		return
	}

	globalPullSecretEnvVar := corev1.EnvVar{
		Name:  hiveconstants.GlobalPullSecret,
		Value: instance.Spec.GlobalPullSecretRef.Name,
	}
	hiveDeployment.Spec.Template.Spec.Containers[0].Env = append(hiveDeployment.Spec.Template.Spec.Containers[0].Env, globalPullSecretEnvVar)
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
