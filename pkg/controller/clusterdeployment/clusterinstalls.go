package clusterdeployment

import (
	"context"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func (r *ReconcileClusterDeployment) reconcileExistingInstallingClusterInstall(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	ref := cd.Spec.ClusterInstallRef
	gvk := schema.GroupVersionKind{
		Group:   ref.Group,
		Version: ref.Version,
		Kind:    ref.Kind,
	}

	logger = logger.WithField("clusterinstall", ref.Name).WithField("gvk", gvk)
	logger.Debug("reconciling existing clusterinstall")

	ci := &hivecontractsv1alpha1.ClusterInstall{}
	err := controllerutils.GetDuckType(context.TODO(), r.Client,
		gvk,
		types.NamespacedName{Namespace: cd.Namespace, Name: ref.Name},
		ci)
	if apierrors.IsNotFound(err) {
		logger.Debug("cluster is not found, so skipping")
		return reconcile.Result{}, nil
	}
	if err != nil {
		logger.WithError(err).Error("failed to get the cluster install")
		return reconcile.Result{}, err
	}

	specModified := false
	statusModified := false
	// copy the cluster metadata
	if met := ci.Spec.ClusterMetadata; met != nil &&
		met.InfraID != "" &&
		met.ClusterID != "" &&
		met.AdminKubeconfigSecretRef.Name != "" &&
		met.AdminPasswordSecretRef != nil &&
		met.AdminPasswordSecretRef.Name != "" {
		if !reflect.DeepEqual(cd.Spec.ClusterMetadata, ci.Spec.ClusterMetadata) {
			cd.Spec.ClusterMetadata = ci.Spec.ClusterMetadata
			specModified = true
		}
	}

	if cd.Status.InstallRestarts != ci.Status.InstallRestarts {
		cd.Status.InstallRestarts = ci.Status.InstallRestarts
		statusModified = true
	}

	conditions := cd.Status.Conditions

	// copy the required conditions
	requiredConditions := []hivev1.ClusterInstallConditionType{
		hivev1.ClusterInstallFailed,
		hivev1.ClusterInstallCompleted,
		hivev1.ClusterInstallStopped,
		hivev1.ClusterInstallRequirementsMet,
	}
	for _, req := range requiredConditions {
		cond := controllerutils.FindCondition(ci.Status.Conditions, req)
		if cond == nil {
			continue
		}

		updated := false
		conditions, updated = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
			hivev1.ClusterDeploymentConditionType("ClusterInstall"+cond.Type), // this transformation is part of the contract
			cond.Status,
			cond.Reason,
			cond.Message,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if updated {
			statusModified = true
		}
	}
	// additionally copy failed to provision failed condition
	// dereference the condition so that the reference isn't clobbered by other updates to the conditions array
	clusterInstallFailed := *controllerutils.FindCondition(conditions, hivev1.ClusterInstallFailedClusterDeploymentCondition)
	updated := false
	conditions, updated = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
		hivev1.ProvisionFailedCondition, // this transformation is part of the contract
		clusterInstallFailed.Status,
		clusterInstallFailed.Reason,
		clusterInstallFailed.Message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if updated {
		statusModified = true
	}

	// take actions based on the conditions
	// like,
	// update install started time when requirements met.
	// update installed = true when completed
	// update the installed timestamp when complete

	clusterInstallRequirementsMet := controllerutils.FindCondition(conditions, hivev1.ClusterInstallRequirementsMetClusterDeploymentCondition)
	if clusterInstallRequirementsMet.Status == corev1.ConditionTrue {
		if !reflect.DeepEqual(cd.Status.InstallStartedTimestamp, &clusterInstallRequirementsMet.LastTransitionTime) {
			cd.Status.InstallStartedTimestamp = &clusterInstallRequirementsMet.LastTransitionTime
			statusModified = true

			kickstartDuration := time.Since(ci.CreationTimestamp.Time)
			logger.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to first provision seconds")
			metricInstallDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
		}
	}

	// dereference the conditions so that the references aren't clobbered by other updates to the conditions array
	clusterInstallStopped := *controllerutils.FindCondition(conditions, hivev1.ClusterInstallStoppedClusterDeploymentCondition)
	clusterInstallCompleted := *controllerutils.FindCondition(conditions, hivev1.ClusterInstallCompletedClusterDeploymentCondition)

	reason := clusterInstallStopped.Reason
	msg := clusterInstallStopped.Message
	if clusterInstallStopped.Status == corev1.ConditionTrue && clusterInstallCompleted.Status == corev1.ConditionFalse && clusterInstallFailed.Status == corev1.ConditionTrue {
		// we must have reached the limit for retrying and therefore
		// gave up with not completed
		reason = installAttemptsLimitReachedReason
		msg = "Install attempts limit reached"
	}

	changedClusterDeploymentStoppedCondition := false
	// Fun extra variable to keep track of whether we should increment metricProvisionFailedTerminal
	// later; because we only want to do that if (we change that status and) the status update succeeds.
	provisionFailedTerminal := false
	conditions, changedClusterDeploymentStoppedCondition = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
		hivev1.ProvisionStoppedCondition,
		clusterInstallStopped.Status,
		reason,
		msg,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if clusterInstallStopped.Status == corev1.ConditionTrue && !changedClusterDeploymentStoppedCondition {
		// they are stopped, we are already stopped, exit early
		return reconcile.Result{}, nil
	}

	// if we are still provisioning...
	if clusterInstallStopped.Status != corev1.ConditionTrue && clusterInstallCompleted.Status != corev1.ConditionTrue && clusterInstallFailed.Status != corev1.ConditionTrue {
		// let the end user know
		conditions, statusModified = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
			hivev1.ProvisionedCondition,
			corev1.ConditionFalse,
			hivev1.ProvisionedReasonProvisioning,
			"Provisioning in progress",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
	}

	// if the cluster install has very recently become stopped...
	if clusterInstallStopped.Status == corev1.ConditionTrue && changedClusterDeploymentStoppedCondition {

		// ...and is also failed...
		if clusterInstallFailed.Status == corev1.ConditionTrue {
			// terminal state, we are not retrying anymore
			statusModified = true
			provisionFailedTerminal = true
			conditions = controllerutils.SetClusterDeploymentCondition(conditions,
				hivev1.ProvisionedCondition,
				corev1.ConditionFalse,
				hivev1.ProvisionedReasonProvisionStopped,
				"Provisioning failed terminally (see the ProvisionStopped condition for details)",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
		}

		// ...and is complete...
		if clusterInstallCompleted.Status == corev1.ConditionTrue {
			// we are done provisioning
			cd.Spec.Installed = true
			cd.Status.InstalledTimestamp = &clusterInstallCompleted.LastTransitionTime
			cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
				cd.Status.Conditions,
				hivev1.ProvisionedCondition,
				corev1.ConditionTrue,
				hivev1.ProvisionedReasonProvisioned,
				"Cluster is provisioned",
				controllerutils.UpdateConditionAlways,
			)
			specModified = true
			statusModified = true

			installStartTime := ci.CreationTimestamp
			if cd.Status.InstallStartedTimestamp != nil {
				installStartTime = *cd.Status.InstallStartedTimestamp // we expect that the install started when requirements met
			}
			installDuration := cd.Status.InstalledTimestamp.Sub(installStartTime.Time)
			logger.WithField("duration", installDuration.Seconds()).Debug("install job completed")
			metricInstallJobDuration.Observe(float64(installDuration.Seconds()))

			metricCompletedInstallJobRestarts.Observe(cd, nil, float64(cd.Status.InstallRestarts))

			metricClustersInstalled.Observe(cd, nil, 1)

			if r.protectedDelete {
				// Set protected delete on for the ClusterDeployment.
				// If the ClusterDeployment already has the ProtectedDelete annotation, do not overwrite it. This allows the
				// user an opportunity to explicitly exclude a ClusterDeployment from delete protection at the time of
				// creation of the ClusterDeployment.
				if _, annotationPresent := cd.Annotations[constants.ProtectedDeleteAnnotation]; !annotationPresent {
					initializeAnnotations(cd)
					cd.Annotations[constants.ProtectedDeleteAnnotation] = "true"
					specModified = true
				}
			}
		}

		// ...and is not failed or completed...
		if clusterInstallFailed.Status != corev1.ConditionTrue && clusterInstallCompleted.Status != corev1.ConditionTrue {
			// terminal state, cluster install contract has been violated
			statusModified = true
			provisionFailedTerminal = true
			conditions = controllerutils.SetClusterDeploymentCondition(conditions,
				hivev1.ProvisionedCondition,
				corev1.ConditionUnknown,
				"Error",
				"Invalid ClusterInstall conditions. Please report this bug.",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
		}
	}

	// if the cluster install is completed without being stopped...
	if clusterInstallStopped.Status != corev1.ConditionTrue && clusterInstallCompleted.Status == corev1.ConditionTrue {
		// terminal state, cluster install contract has been violated
		statusModified = true
		provisionFailedTerminal = true
		conditions = controllerutils.SetClusterDeploymentCondition(conditions,
			hivev1.ProvisionedCondition,
			corev1.ConditionUnknown,
			"Error",
			"Invalid ClusterInstall conditions. Please report this bug.",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
	}

	if specModified {
		if err := r.Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Error("failed to update the spec of clusterdeployment")
			return reconcile.Result{}, err
		}
	}
	if statusModified {
		cd.Status.Conditions = conditions
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Error("failed to update the status of clusterdeployment")
			return reconcile.Result{}, err
		}
		// If we declared the provision terminally failed, bump our metric
		if provisionFailedTerminal {
			incProvisionFailedTerminal(cd)
		}
	}

	return reconcile.Result{}, nil
}

func getClusterImageSetFromClusterInstall(client client.Client, cd *hivev1.ClusterDeployment) (string, error) {
	ref := cd.Spec.ClusterInstallRef
	gvk := schema.GroupVersionKind{
		Group:   ref.Group,
		Version: ref.Version,
		Kind:    ref.Kind,
	}

	ci := &hivecontractsv1alpha1.ClusterInstall{}
	err := controllerutils.GetDuckType(context.TODO(), client,
		gvk,
		types.NamespacedName{Namespace: cd.Namespace, Name: ref.Name},
		ci)
	if err != nil {
		return "", err
	}
	return ci.Spec.ImageSetRef.Name, nil
}

const clusterInstallIndexFieldName = "spec.clusterinstalls"

func indexClusterInstall(o client.Object) []string {
	var res []string
	cd := o.(*hivev1.ClusterDeployment)

	if cd.Spec.ClusterInstallRef != nil {
		res = append(res, cd.Spec.ClusterInstallRef.Name)
	}

	return res
}

func (r *ReconcileClusterDeployment) watchClusterInstall(gvk schema.GroupVersionKind, logger log.FieldLogger) error {
	_, ok := r.watchingClusterInstall[gvk.String()]
	if ok {
		return nil
	}

	logger.WithField("gvk", gvk).Debug("adding cluster install watches")

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	err := r.watcher.Watch(source.Kind(r.Manager.GetCache(), obj), handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		retval := []reconcile.Request{}

		cdList := &hivev1.ClusterDeploymentList{}
		if err := r.Client.List(context.TODO(), cdList,
			client.MatchingFields{clusterInstallIndexFieldName: o.GetName()},
			client.InNamespace(o.GetNamespace())); err != nil {
			logger.WithError(err).Error("failed to list cluster deployment matching cluster install index")
			return retval
		}

		for _, cd := range cdList.Items {
			retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: cd.Namespace,
				Name:      cd.Name,
			}})
		}
		logger.WithField("retval", retval).Debug("trigger reconcile for clusterdeployments for cluster install objects")

		return retval
	}))
	if err != nil {
		return err
	}

	logger.WithField("gvk", gvk).Debug("added new watcher for cluster install")
	r.watchingClusterInstall[gvk.String()] = struct{}{}
	return nil
}
