package clusterdeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	installertypes "github.com/openshift/installer/pkg/types"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
)

type ClusterProvisionManager struct{}

func (r *ReconcileClusterDeployment) startNewProvision(
	cd *hivev1.ClusterDeployment,
	releaseImage string,
	logger log.FieldLogger,
) (result reconcile.Result, returnedErr error) {
	// Preflight; If we've reached ProvisionStopped for any reason, bail
	if pfcond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); pfcond != nil && pfcond.Status == corev1.ConditionTrue {
		logger.Debug("ProvisionStopped is True; not creating a new provision")
		return reconcile.Result{}, nil
	}

	existingProvisions, err := r.existingProvisions(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	var lastFailedProvision *hivev1.ClusterProvision
	for _, lastFailedProvision = range existingProvisions {
		if lastFailedProvision.Spec.Stage != hivev1.ClusterProvisionStageFailed {
			return reconcile.Result{}, r.adoptProvision(cd, lastFailedProvision, logger)
		}
	}
	// If we get here, `lastFailedProvision` is either nil (no provisions exist yet) or points to
	// the most recent (because `existingProvisions` is sorted by age) failed (because otherwise we
	// went to adoptProvision) ClusterProvision.

	// ...and this is guaranteed not to delete the above, as long as we continue to save >2 failed
	// provisions (the first is always the oldest).
	r.deleteStaleProvisions(existingProvisions, logger)

	setProvisionStoppedTrue := func(reason, message string) (reconcile.Result, error) {
		logger.Debugf("not creating new provision: %s", message)
		var changed1, changed2 bool
		var conditions []hivev1.ClusterDeploymentCondition
		conditions, changed1 = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.ProvisionStoppedCondition,
			corev1.ConditionTrue,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)

		conditions, changed2 = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.ProvisionedCondition,
			corev1.ConditionFalse,
			hivev1.ProvisionedReasonProvisionStopped,
			"Provisioning failed terminally (see the ProvisionStopped condition for details)",
			controllerutils.UpdateConditionIfReasonOrMessageChange)

		if changed1 || changed2 {
			cd.Status.Conditions = conditions
			logger.Debugf("setting ProvisionStoppedCondition to %v", corev1.ConditionTrue)
			if err := r.Status().Update(context.TODO(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
				return reconcile.Result{}, err
			}
			incProvisionFailedTerminal(cd)
		}
		return reconcile.Result{}, nil
	}

	if cd.Status.InstallRestarts > 0 && cd.Annotations[tryInstallOnceAnnotation] == "true" {
		return setProvisionStoppedTrue(installOnlyOnceSetReason, "Deployment is set to try install only once")
	}
	if cd.Spec.InstallAttemptsLimit != nil && cd.Status.InstallRestarts >= int(*cd.Spec.InstallAttemptsLimit) {
		return setProvisionStoppedTrue(installAttemptsLimitReachedReason, "Install attempts limit reached")
	}
	shouldRetry, err := r.shouldRetryBasedOnFailureReason(lastFailedProvision, logger)
	if err != nil {
		logger.WithError(err).Error("failed to determine whether to retry based on provision failure reason")
		return reconcile.Result{}, err
	}
	if !shouldRetry {
		return setProvisionStoppedTrue(failureReasonNotListed, "Provision failure reason not retryable")
	}

	conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ProvisionStoppedCondition,
		corev1.ConditionFalse,
		provisionNotStoppedReason,
		"Provision is not stopped",
		controllerutils.UpdateConditionNever)
	if changed {
		cd.Status.Conditions = conditions
		logger.Debugf("setting ProvisionStoppedCondition to %v", corev1.ConditionFalse)
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if err := controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error setting up service account and role")
		return reconcile.Result{}, err
	}

	provisionName := apihelpers.GetResourceName(cd.Name, fmt.Sprintf("%d-%s", cd.Status.InstallRestarts, utilrand.String(5)))

	labels := cd.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constants.ClusterDeploymentNameLabel] = cd.Name

	extraEnvVars, err := getInstallLogEnvVars(cd.Name)
	if err != nil {
		logger.WithError(err).Error("failed to read failed provision config file")
		return reconcile.Result{}, err
	}
	extraEnvVars = append(extraEnvVars, getAWSServiceProviderEnvVars(cd, cd.Name)...)

	podSpec, err := install.InstallerPodSpec(
		cd,
		provisionName,
		releaseImage,
		controllerutils.InstallServiceAccountName,
		os.Getenv("HTTP_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("NO_PROXY"),
		extraEnvVars,
	)
	if err != nil {
		logger.WithError(err).Error("could not generate installer pod spec")
		return reconcile.Result{}, err
	}

	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisionName,
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			PodSpec: *podSpec,
			Attempt: cd.Status.InstallRestarts,
			Stage:   hivev1.ClusterProvisionStageInitializing,
		},
	}
	controllerutils.CopyLogAnnotation(cd, provision)

	// Copy over the name, cluster ID and infra ID from previous provision so that a failed install can be removed.
	if lastFailedProvision != nil {
		provision.Spec.PrevProvisionName = &lastFailedProvision.Name
		provision.Spec.PrevClusterID = lastFailedProvision.Spec.ClusterID
		provision.Spec.PrevInfraID = lastFailedProvision.Spec.InfraID
	}

	logger.WithField("derivedObject", provision.Name).Debug("Setting label on derived object")
	provision.Labels = k8slabels.AddLabel(provision.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
	if err := controllerutil.SetControllerReference(cd, provision, r.scheme); err != nil {
		logger.WithError(err).Error("could not set the owner ref on provision")
		return reconcile.Result{}, err
	}

	if err := r.copyInstallLogSecret(provision.Namespace, extraEnvVars); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			// Couldn't copy the install log secret for a reason other than it already exists.
			// If the secret already exists, then we should just use that secret.
			logger.WithError(err).Error("could not copy install log secret")
			return reconcile.Result{}, err
		}
	}

	if err := install.CopyAWSServiceProviderSecret(r.Client, provision.Namespace, extraEnvVars, cd, r.scheme); err != nil {
		logger.WithError(err).Error("could not copy AWS service provider secret")
		return reconcile.Result{}, err
	}

	if err := r.setupAWSCredentialForAssumeRole(cd); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			// Couldn't create the assume role credential secret for a reason other than it already exists.
			// If the secret already exists, then we should just use that secret.
			logger.WithError(err).Error("could not create AWS assume role credential secret")
			return reconcile.Result{}, err
		}
	}

	r.expectations.ExpectCreations(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(), 1)
	if err := r.Create(context.TODO(), provision); err != nil {
		logger.WithError(err).Error("could not create provision")
		r.expectations.CreationObserved(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String())
		return reconcile.Result{}, err
	}

	logger.WithField("provision", provision.Name).Info("created new provision")

	if err := r.updateCondition(
		cd,
		hivev1.ProvisionedCondition,
		corev1.ConditionFalse,
		hivev1.ProvisionedReasonProvisioning,
		"Cluster provision created",
		logger,
	); err != nil {
		return reconcile.Result{}, err
	}

	if cd.Status.InstallRestarts == 0 {
		kickstartDuration := time.Since(cd.CreationTimestamp.Time)
		logger.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to first provision seconds")
		metricInstallDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) shouldRetryBasedOnFailureReason(prov *hivev1.ClusterProvision, logger log.FieldLogger) (bool, error) {
	// prov will be nil if there have been no failed provisions yet
	if prov == nil {
		logger.Debug("no failed provisions yet -- allowing retry")
		return true, nil
	}
	// Load up FailedProvisionConfig
	fpConfig, err := readProvisionFailedConfig()
	if err != nil {
		return false, err
	}
	// If no retry reasons are specified, "always" retry
	if fpConfig.RetryReasons == nil {
		logger.Debug("no RetryReasons found in FailedProvisionConfig -- allowing retry")
		return true, nil
	}
	// Does our failed provision's reason match?
	cond := controllerutils.FindCondition(
		prov.Status.Conditions, hivev1.ClusterProvisionFailedCondition)
	if cond == nil {
		return false, errors.New("failed to find ClusterProvisionFailed Condition -- this should never happen!")
	}
	rLog := logger.WithField("reason", cond.Reason)
	for _, reason := range *fpConfig.RetryReasons {
		if cond.Reason == reason {
			rLog.Debug("retrying due to matching retry reason")
			return true, nil
		}
	}
	rLog.Debug("reason not found in FailedProvisionConfig RetryReasons -- not retrying")
	return false, nil
}

// setAzureResourceGroupFromMetadata unmarshals `pm.Raw`, a representation of the installer ClusterMetadata type,
// and looks for the Azure ResourceGroupName therein. If found, the value is copied into the Azure platform-specific
// section of `cm`, hive's representation of the cluster metadata. The `cd` is only used to validate that we're
// operating on an Azure cluster.
// The caller is responsible for copying `cm` back into `cd` and Update()ing if/as necessary.
func setAzureResourceGroupFromMetadata(cd *hivev1.ClusterDeployment, cm *hivev1.ClusterMetadata, pmjson []byte, logger log.FieldLogger) {
	if pmjson == nil {
		return
	}
	if cd.Spec.Platform.Azure == nil {
		return
	}

	im := new(installertypes.ClusterMetadata)
	if err := json.Unmarshal(pmjson, &im); err != nil {
		logger.WithError(err).Error("Could not unmarshal ClusterMetadata!")
		return
	}

	if im.Azure == nil {
		logger.Warn("ClusterMetadata unexpectedly has no Azure section")
		return
	}
	rg := im.Azure.ResourceGroupName
	if rg != "" {
		log.WithField("resourceGroupName", rg).Info("Found Azure ResourceGroupName in ClusterMetadata")
	} else {
		// Default it if possible
		if im.InfraID == "" {
			// This shouldn't be possible.
			log.Warn("Can't set default Azure ResourceGroup yet: no InfraID set. This should not happen.")
			return
		}
		// This is the default set by the installer
		rg = fmt.Sprintf("%s-rg", im.InfraID)
		log.WithField("resourceGroupName", rg).Info("Azure ResourceGroupName unset in ClusterMetadata; defaulting")
	}

	if cm.Platform == nil {
		cm.Platform = &hivev1.ClusterPlatformMetadata{}
	}
	if cm.Platform.Azure == nil {
		cm.Platform.Azure = &azure.Metadata{}
	}
	cm.Platform.Azure.ResourceGroupName = &rg
}

func (r *ReconcileClusterDeployment) reconcileExistingProvision(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (result reconcile.Result, returnedErr error) {
	logger = logger.WithField("provision", cd.Status.ProvisionRef.Name)
	logger.Debug("reconciling existing provision")

	provision := &hivev1.ClusterProvision{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.ProvisionRef.Name, Namespace: cd.Namespace}, provision); {
	case apierrors.IsNotFound(err):
		logger.Warn("linked provision not found")
		return r.clearOutCurrentProvision(cd, logger)
	case err != nil:
		logger.WithError(err).Error("could not get provision")
		return reconcile.Result{}, err
	}

	// Save the cluster ID and infra ID from the provision so that we can
	// clean up partial installs on the next provision attempt in case of failure.
	if provision.Spec.InfraID != nil {
		clusterMetadata := &hivev1.ClusterMetadata{}
		clusterMetadata.InfraID = *provision.Spec.InfraID
		if provision.Spec.ClusterID != nil {
			clusterMetadata.ClusterID = *provision.Spec.ClusterID
		}
		if provision.Spec.AdminKubeconfigSecretRef != nil {
			clusterMetadata.AdminKubeconfigSecretRef = *provision.Spec.AdminKubeconfigSecretRef
		}
		if provision.Spec.AdminPasswordSecretRef != nil {
			clusterMetadata.AdminPasswordSecretRef = provision.Spec.AdminPasswordSecretRef
		}
		setAzureResourceGroupFromMetadata(cd, clusterMetadata, provision.Spec.MetadataJSON, logger)
		if !reflect.DeepEqual(clusterMetadata, cd.Spec.ClusterMetadata) {
			cd.Spec.ClusterMetadata = clusterMetadata
			logger.Infof("Saving infra ID %q for cluster", cd.Spec.ClusterMetadata.InfraID)
			err := r.Update(context.TODO(), cd)
			if err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "error updating clusterdeployment status with infra ID")
			}
			return reconcile.Result{}, err
		}
	}

	switch provision.Spec.Stage {
	case hivev1.ClusterProvisionStageInitializing:
		return r.reconcileInitializingProvision(cd, provision, logger)
	case hivev1.ClusterProvisionStageProvisioning:
		return r.reconcileProvisioningProvision(cd, provision, logger)
	case hivev1.ClusterProvisionStageFailed:
		return r.reconcileFailedProvision(cd, provision, logger)
	case hivev1.ClusterProvisionStageComplete:
		return r.reconcileCompletedProvision(cd, provision, logger)
	default:
		logger.WithField("stage", provision.Spec.Stage).Error("unknown provision stage")
		return reconcile.Result{}, errors.New("unknown provision stage")
	}
}

func (r *ReconcileClusterDeployment) stopProvisioning(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (*reconcile.Result, error) {
	if cd.Status.ProvisionRef == nil {
		return nil, nil
	}
	provision := &hivev1.ClusterProvision{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.ProvisionRef.Name, Namespace: cd.Namespace}, provision); {
	case apierrors.IsNotFound(err):
		logger.Debug("linked provision removed")
		return nil, nil
	case err != nil:
		logger.WithError(err).Error("could not get provision")
		return nil, err
	case provision.DeletionTimestamp == nil:
		if err := r.Delete(context.TODO(), provision); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not delete provision")
			return nil, err
		}
		logger.Info("deleted outstanding provision")
		return &reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
	default:
		logger.Debug("still waiting for outstanding provision to be removed")
		return &reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
	}
}

func (r *ReconcileClusterDeployment) reconcileInitializingProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	cdLog.Debug("still initializing provision")
	// Set condition on ClusterDeployment when install pod is stuck in pending phase
	installPodStuckCondition := controllerutils.FindCondition(provision.Status.Conditions, hivev1.InstallPodStuckCondition)
	if installPodStuckCondition != nil && installPodStuckCondition.Status == corev1.ConditionTrue {
		if err := r.updateCondition(cd, hivev1.InstallLaunchErrorCondition, corev1.ConditionTrue, installPodStuckCondition.Reason, installPodStuckCondition.Message, cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update InstallLaunchErrorCondition")
			return reconcile.Result{}, err
		}
	}
	if err := r.updateCondition(
		cd,
		hivev1.ProvisionedCondition,
		corev1.ConditionFalse,
		hivev1.ProvisionedReasonProvisioning,
		"Cluster provision initializing",
		cdLog,
	); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) reconcileProvisioningProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	cdLog.Debug("still provisioning")
	if err := r.updateCondition(cd, hivev1.InstallLaunchErrorCondition, corev1.ConditionFalse, "InstallLaunchSuccessful", "Successfully launched install pod", cdLog); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update InstallLaunchErrorCondition")
		return reconcile.Result{}, err
	}
	if err := r.updateCondition(
		cd,
		hivev1.ProvisionedCondition,
		corev1.ConditionFalse,
		hivev1.ProvisionedReasonProvisioning,
		"Cluster is provisioning",
		cdLog,
	); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) reconcileFailedProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	nextProvisionTime := time.Now()
	reason := "MissingCondition"
	message := fmt.Sprintf("Provision %s failed. Next provision will begin soon.", provision.Name)

	failedCond := controllerutils.FindCondition(provision.Status.Conditions, hivev1.ClusterProvisionFailedCondition)
	if failedCond != nil && failedCond.Status == corev1.ConditionTrue {
		nextProvisionTime = calculateNextProvisionTime(failedCond.LastTransitionTime.Time, cd.Status.InstallRestarts, cdLog)
		reason = failedCond.Reason
		message = failedCond.Message
	} else {
		cdLog.Warnf("failed provision does not have a %s condition", hivev1.ClusterProvisionFailedCondition)
	}

	newConditions, condChange := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ProvisionFailedCondition,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	cd.Status.Conditions = newConditions

	timeUntilNextProvision := time.Until(nextProvisionTime)
	if timeUntilNextProvision.Seconds() > 0 {
		cdLog.WithField("nextProvision", nextProvisionTime).Info("waiting to start a new provision after failure")
		if condChange {
			if err := r.statusUpdate(cd, cdLog); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{RequeueAfter: timeUntilNextProvision}, nil
	}

	cdLog.Info("clearing current failed provision to make way for a new provision")
	return r.clearOutCurrentProvision(cd, cdLog)
}

func (r *ReconcileClusterDeployment) reconcileCompletedProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	cdLog.Info("provision completed successfully")

	statusChange := false
	if cd.Status.InstalledTimestamp == nil {
		statusChange = true
		now := metav1.Now()
		cd.Status.InstalledTimestamp = &now
	}
	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ProvisionFailedCondition,
		corev1.ConditionFalse,
		"ProvisionSucceeded",
		fmt.Sprintf("Provision %s succeeded.", provision.Name),
		controllerutils.UpdateConditionNever,
	)
	if changed {
		statusChange = true
		cd.Status.Conditions = conds
	}
	conds, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ProvisionedCondition,
		corev1.ConditionTrue,
		hivev1.ProvisionedReasonProvisioned,
		"Cluster is provisioned",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
		statusChange = true
		cd.Status.Conditions = conds
	}
	if statusChange {
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
	}

	if cd.Spec.Installed {
		return reconcile.Result{}, nil
	}

	cd.Spec.Installed = true

	if r.protectedDelete {
		// Set protected delete on for the ClusterDeployment.
		// If the ClusterDeployment already has the ProtectedDelete annotation, do not overwrite it. This allows the
		// user an opportunity to explicitly exclude a ClusterDeployment from delete protection at the time of
		// creation of the ClusterDeployment.
		if _, annotationPresent := cd.Annotations[constants.ProtectedDeleteAnnotation]; !annotationPresent {
			initializeAnnotations(cd)
			cd.Annotations[constants.ProtectedDeleteAnnotation] = "true"
		}
	}

	if err := r.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to set the Installed flag")
		return reconcile.Result{}, err
	}

	// jobDuration calculates the time elapsed since the first clusterprovision was created
	startTime := cd.CreationTimestamp
	if firstProvision := r.getFirstProvision(cd, cdLog); firstProvision != nil {
		startTime = firstProvision.CreationTimestamp
	}
	jobDuration := time.Since(startTime.Time)
	cdLog.WithField("duration", jobDuration.Seconds()).Debug("install job completed")
	metricInstallJobDuration.Observe(float64(jobDuration.Seconds()))

	// Report a metric for the total number of install restarts:
	metricCompletedInstallJobRestarts.Observe(cd, nil, float64(cd.Status.InstallRestarts))

	metricClustersInstalled.Observe(cd, nil, 1)

	return reconcile.Result{}, nil
}

func getClusterImageSetFromProvisioning(cd *hivev1.ClusterDeployment) string {
	if cd.Spec.Provisioning.ImageSetRef != nil {
		return cd.Spec.Provisioning.ImageSetRef.Name
	}
	return ""
}

func (r *ReconcileClusterDeployment) clearOutCurrentProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	cd.Status.ProvisionRef = nil
	cd.Status.InstallRestarts = cd.Status.InstallRestarts + 1
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not clear out current provision")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) copyInstallLogSecret(destNamespace string, extraEnvVars []corev1.EnvVar) error {
	hiveNS := controllerutils.GetHiveNamespace()

	srcSecretName, foundSrc := os.LookupEnv(constants.InstallLogsCredentialsSecretRefEnvVar)
	if !foundSrc {
		// If the src secret reference wasn't found, then don't attempt to copy the secret.
		return nil
	}

	foundDest := false
	var destSecretName string
	for _, envVar := range extraEnvVars {
		if envVar.Name == constants.InstallLogsCredentialsSecretRefEnvVar {
			destSecretName = envVar.Value
			foundDest = true
		}
	}

	if !foundDest {
		// If the dest secret reference wasn't found, then don't attempt to copy the secret.
		return nil
	}

	src := types.NamespacedName{Name: srcSecretName, Namespace: hiveNS}
	dest := types.NamespacedName{Name: destSecretName, Namespace: destNamespace}
	return controllerutils.CopySecret(r, src, dest, nil, nil)
}

// NOTE: Ugly-but-simple way to mock os.ReadFile for test purposes.
// https://stackoverflow.com/questions/20923938/how-would-i-mock-a-call-to-ioutil-readfile-in-go/37035375
// This variable is overridden by fakeReadFile.
var readFile = os.ReadFile

// readProvisionFailedConfig reads the provision fail config from the file pointed to
// by the FailedProvisionConfigFileEnvVar environment variable.
func readProvisionFailedConfig() (*hivev1.FailedProvisionConfig, error) {
	path := os.Getenv(constants.FailedProvisionConfigFileEnvVar)
	config := &hivev1.FailedProvisionConfig{}
	if len(path) == 0 {
		return config, nil
	}

	fileBytes, err := readFile(path)
	if err != nil || len(fileBytes) == 0 {
		return config, err
	}
	if err := json.Unmarshal(fileBytes, config); err != nil {
		return config, err
	}

	return config, nil
}

func getInstallLogEnvVars(secretPrefix string) ([]corev1.EnvVar, error) {
	var extraEnvVars = []corev1.EnvVar{}
	fpConfig, err := readProvisionFailedConfig()
	if err != nil || fpConfig == nil {
		return extraEnvVars, err
	}
	if awsSpec := fpConfig.AWS; awsSpec != nil {
		// By default we will try to gather logs on failed installs:
		extraEnvVars = []corev1.EnvVar{
			{
				Name:  constants.InstallLogsUploadProviderEnvVar,
				Value: constants.InstallLogsUploadProviderAWS,
			},
			{
				Name:  constants.InstallLogsCredentialsSecretRefEnvVar,
				Value: secretPrefix + "-" + awsSpec.CredentialsSecretRef.Name,
			},
			{
				Name:  constants.InstallLogsAWSRegionEnvVar,
				Value: awsSpec.Region,
			},
			{
				Name:  constants.InstallLogsAWSServiceEndpointEnvVar,
				Value: awsSpec.ServiceEndpoint,
			},
			{
				Name:  constants.InstallLogsAWSS3BucketEnvVar,
				Value: awsSpec.Bucket,
			},
		}
	}

	return extraEnvVars, nil
}

func getAWSServiceProviderEnvVars(cd *hivev1.ClusterDeployment, secretPrefix string) []corev1.EnvVar {
	var extraEnvVars []corev1.EnvVar
	spSecretName := os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar)
	if spSecretName == "" {
		return extraEnvVars
	}

	if cd.Spec.Platform.AWS == nil {
		return extraEnvVars
	}

	extraEnvVars = append(extraEnvVars, corev1.EnvVar{
		Name:  constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar,
		Value: secretPrefix + "-" + spSecretName,
	})
	return extraEnvVars
}

func (r *ReconcileClusterDeployment) setupAWSCredentialForAssumeRole(cd *hivev1.ClusterDeployment) error {
	if cd.Spec.Platform.AWS == nil ||
		cd.Spec.Platform.AWS.CredentialsSecretRef.Name != "" ||
		cd.Spec.Platform.AWS.CredentialsAssumeRole == nil {
		// no setup required
		return nil
	}

	return install.AWSAssumeRoleCLIConfig(r.Client, cd.Spec.Platform.AWS.CredentialsAssumeRole, install.AWSAssumeRoleSecretName(cd.Name), cd.Namespace, cd, r.scheme)
}

func (r *ReconcileClusterDeployment) watchClusterProvisions(mgr manager.Manager, c controller.Controller) error {
	handler := &clusterProvisionEventHandler{
		EventHandler: handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()),
		reconciler:   r,
	}
	return c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterProvision{}), handler)
}

var _ handler.EventHandler = &clusterProvisionEventHandler{}

type clusterProvisionEventHandler struct {
	handler.EventHandler
	reconciler *ReconcileClusterDeployment
}

// Create implements handler.EventHandler
func (h *clusterProvisionEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("ClusterProvision created")
	h.reconciler.trackClusterProvisionAdd(e.Object)
	h.EventHandler.Create(ctx, e, q)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileClusterDeployment) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *hivev1.ClusterDeployment {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	cd := &hivev1.ClusterDeployment{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}, cd); err != nil {
		return nil
	}
	if cd.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return cd
}

// When a clusterprovision is created, update the expectations of the clusterdeployment that owns the clusterprovision.
func (r *ReconcileClusterDeployment) trackClusterProvisionAdd(obj interface{}) {
	provision := obj.(*hivev1.ClusterProvision)
	if provision.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(provision); controllerRef != nil {
		cd := r.resolveControllerRef(provision.Namespace, controllerRef)
		if cd == nil {
			return
		}
		cdKey := types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String()
		r.expectations.CreationObserved(cdKey)
	}
}
