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
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
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
	requiredConditions := []string{
		hivev1.ClusterInstallFailed,
		hivev1.ClusterInstallCompleted,
		hivev1.ClusterInstallStopped,
		hivev1.ClusterInstallRequirementsMet,
	}
	for _, req := range requiredConditions {
		cond := controllerutils.FindClusterInstallCondition(ci.Status.Conditions, req)
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
	failed := controllerutils.FindClusterDeploymentCondition(conditions, hivev1.ClusterInstallFailedClusterDeploymentCondition)
	if failed != nil {
		updated := false
		conditions, updated = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
			hivev1.ProvisionFailedCondition, // this transformation is part of the contract
			failed.Status,
			failed.Reason,
			failed.Message,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if updated {
			statusModified = true
		}
	}

	// take actions based on the conditions
	// like,
	// update install started time when requirements met.
	// update installed = true when completed
	// update the installed timestamp when complete

	requirementsMet := controllerutils.FindClusterDeploymentCondition(conditions, hivev1.ClusterInstallRequirementsMetClusterDeploymentCondition)
	if requirementsMet != nil &&
		requirementsMet.Status == corev1.ConditionTrue {
		if !reflect.DeepEqual(cd.Status.InstallStartedTimestamp, &requirementsMet.LastTransitionTime) {
			cd.Status.InstallStartedTimestamp = &requirementsMet.LastTransitionTime
			statusModified = true

			kickstartDuration := time.Since(ci.CreationTimestamp.Time)
			logger.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to first provision seconds")
			metricInstallDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
		}
	}

	completed := controllerutils.FindClusterDeploymentCondition(conditions, hivev1.ClusterInstallCompletedClusterDeploymentCondition)
	stopped := controllerutils.FindClusterDeploymentCondition(conditions, hivev1.ClusterInstallStoppedClusterDeploymentCondition)

	if stopped != nil {
		reason := stopped.Reason
		msg := stopped.Message
		if stopped.Status == corev1.ConditionTrue &&
			(completed == nil || completed.Status != corev1.ConditionTrue) {
			// we are must have reached the limit for retrying and therefore
			// gave up with not completed
			reason = installAttemptsLimitReachedReason
			msg = "Install attempts limit reached"
		}

		updated := false
		conditions, updated = controllerutils.SetClusterDeploymentConditionWithChangeCheck(conditions,
			hivev1.ProvisionStoppedCondition,
			stopped.Status,
			reason,
			msg,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if updated {
			statusModified = true
		}
	}

	if completed != nil && completed.Status == corev1.ConditionTrue { // the cluster install is complete
		cd.Spec.Installed = true
		cd.Status.InstalledTimestamp = &completed.LastTransitionTime
		specModified = true
		statusModified = true

		installStartTime := ci.CreationTimestamp
		if cd.Status.InstallStartedTimestamp != nil {
			installStartTime = *cd.Status.InstallStartedTimestamp // we expect that the install started when requirements met
		}
		installDuration := cd.Status.InstalledTimestamp.Sub(installStartTime.Time)
		logger.WithField("duration", installDuration.Seconds()).Debug("install job completed")
		metricInstallJobDuration.Observe(float64(installDuration.Seconds()))

		metricCompletedInstallJobRestarts.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).
			Observe(float64(cd.Status.InstallRestarts))

		metricClustersInstalled.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()

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
	err := r.watcher.Watch(&source.Kind{Type: obj}, handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
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
