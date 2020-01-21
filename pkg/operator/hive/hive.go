package hive

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"strconv"

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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dnsServersEnvVar = "ZONE_CHECK_DNS_SERVERS"

	// hiveAdditionalCASecret is the name of the secret in the hive namespace
	// that will contain the aggregate of all AdditionalCertificateAuthorities
	// secrets specified in HiveConfig
	hiveAdditionalCASecret = "hive-additional-ca"
)

func (r *ReconcileHiveConfig) deployHive(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, recorder events.Recorder) error {

	asset := assets.MustAsset("config/manager/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveContainer := &hiveDeployment.Spec.Template.Spec.Containers[0]

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

	if len(instance.Spec.ManagedDomains) > 0 {
		addManagedDomainsVolume(&hiveDeployment.Spec.Template.Spec)
	}

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

	result, err := h.ApplyRuntimeObject(hiveDeployment, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("deployment applied (%s)", result)

	applyAssets := []string{
		"config/manager/service.yaml",

		"config/rbac/hive_frontend_role.yaml",
		"config/rbac/hive_frontend_role_binding.yaml",
		"config/rbac/hive_frontend_serviceaccount.yaml",

		// Due to bug with OLM not updating CRDs on upgrades, we are re-applying
		// the latest in the operator to ensure updates roll out.
		"config/crds/hive_v1_clusterdeployment.yaml",
		"config/crds/hive_v1_clusterdeprovision.yaml",
		"config/crds/hive_v1_clusterimageset.yaml",
		"config/crds/hive_v1_dnszone.yaml",
		"config/crds/hive_v1_hiveconfig.yaml",
		"config/crds/hive_v1_selectorsyncidentityprovider.yaml",
		"config/crds/hive_v1_selectorsyncset.yaml",
		"config/crds/hive_v1_syncidentityprovider.yaml",
		"config/crds/hive_v1_syncset.yaml",

		"config/configmaps/install-log-regexes-configmap.yaml",
	}

	// In very rare cases we use OpenShift specific types which will not apply if running on
	// vanilla Kubernetes. Detect this and skip if so.
	openshiftSpecificAssets := []string{
		"config/rbac/hive_admin_role.yaml",
		"config/rbac/hive_admin_role_binding.yaml",
		"config/rbac/hive_reader_role.yaml",
		"config/rbac/hive_reader_role_binding.yaml",
	}

	for _, a := range applyAssets {
		err = util.ApplyAsset(h, a, hLog)
		if err != nil {
			return err
		}
	}

	if r.runningOnOpenShift(hLog) {
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

	// Remove legacy ClusterImageSets we do not want installable anymore.
	removeImageSets := []string{
		"openshift-v4.0-beta3",
		"openshift-v4.0-beta4",
		"openshift-v4.0-latest",
	}
	for _, isName := range removeImageSets {
		clusterImageSet := &hivev1.ClusterImageSet{}
		err := r.Get(context.Background(), types.NamespacedName{Name: isName}, clusterImageSet)
		if err != nil && !errors.IsNotFound(err) {
			hLog.WithError(err).Error("error looking for obsolete ClusterImageSet")
			return err
		} else if err != nil {
			hLog.WithField("clusterImageSet", isName).Debug("legacy ClusterImageSet does not exist")
		} else {
			err = r.Delete(context.Background(), clusterImageSet)
			if err != nil {
				hLog.WithError(err).WithField("clusterImageSet", clusterImageSet).Error(
					"error deleting outdated ClusterImageSet")
				return err
			}
			hLog.WithField("clusterImageSet", isName).Info("deleted outdated ClusterImageSet")
		}

	}

	hLog.Info("all hive components successfully reconciled")
	return nil
}

func (r *ReconcileHiveConfig) includeAdditionalCAs(hLog log.FieldLogger, h *resource.Helper, instance *hivev1.HiveConfig, hiveDeployment *appsv1.Deployment) error {
	additionalCA := &bytes.Buffer{}
	for _, clientCARef := range instance.Spec.AdditionalCertificateAuthoritiesSecretRef {
		caSecret := &corev1.Secret{}
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: constants.HiveNamespace, Name: clientCARef.Name}, caSecret)
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
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: constants.HiveNamespace, Name: hiveAdditionalCASecret}, caSecret)
		if err == nil {
			err = r.Delete(context.TODO(), caSecret)
			if err != nil {
				hLog.WithError(err).WithField("secret", fmt.Sprintf("%s/%s", constants.HiveNamespace, hiveAdditionalCASecret)).
					Error("cannot delete hive additional ca secret")
				return err
			}
		}
		return nil
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.HiveNamespace,
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

func (r *ReconcileHiveConfig) runningOnOpenShift(hLog log.FieldLogger) bool {
	// DeploymentConfig is an OpenShift specific type we have go types vendored for, see
	// if we can list them to determine if we're running on OpenShift or vanilla Kube.
	dcs := &oappsv1.DeploymentConfigList{}
	err := r.List(context.Background(), dcs, client.InNamespace(constants.HiveNamespace))
	if err != nil {
		hLog.WithError(err).Debug("error listing DeploymentConfig to determine if running on OpenShift")
	}
	return err == nil
}
