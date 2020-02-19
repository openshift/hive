package labelobjects

import (
	"context"
	"fmt"
	"os"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	"github.com/pkg/errors"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"

	hivecontrolplanecerts "github.com/openshift/hive/pkg/controller/controlplanecerts"
	hiveremoteingress "github.com/openshift/hive/pkg/controller/remoteingress"
	hivesyncidentityprovider "github.com/openshift/hive/pkg/controller/syncidentityprovider"
	hiveimageset "github.com/openshift/hive/pkg/imageset"
)

// Options contains the options for labeling related Hive objects
type Options struct {
	Logger logger.FieldLogger
	DryRun bool
}

// NewLabelObjectsCommand adds a subcommand for labeling Hive objects in a cluster.
func NewLabelObjectsCommand() *cobra.Command {
	opt := &Options{}
	logLevel := "info"
	cmd := &cobra.Command{
		Use:   "label-objects",
		Short: "Labels related Hive objects in the cluster",
		Run: func(cmd *cobra.Command, args []string) {
			// Set log level
			level, err := logger.ParseLevel(logLevel)
			if err != nil {
				logger.WithError(err).Error("Cannot parse log level")
				os.Exit(1)
			}

			log := logger.NewEntry(&logger.Logger{
				Out: os.Stdout,
				Formatter: &logger.TextFormatter{
					FullTimestamp: true,
				},
				Hooks: make(logger.LevelHooks),
				Level: level,
			})
			opt.Logger = log

			kubeclient, err := contributils.GetClient()
			if err != nil {
				log.WithError(err).Fatal("error creating kube clients")
			}

			if _, err := opt.run(kubeclient); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.BoolVar(&opt.DryRun, "dry-run", false, "Determines if the migration should change values in the cluster")

	return cmd
}

type labelMigrator struct {
	client    client.Client
	dryRun    bool
	logger    logger.FieldLogger
	succeeded bool

	clusterProvisionsSuccessCount               int
	installLogPVCsSuccessCount                  int
	imagesetJobsSuccessCount                    int
	clusterDeprovisionsSuccessCount             int
	dnsZonesSuccessCount                        int
	mergedPullSecretsSuccessCount               int
	clusterDeprovisionJobsSuccessCount          int
	clusterProvisionJobsSuccessCount            int
	clusterStatesSuccessCount                   int
	controlPlaneCertificateSyncSetsSuccessCount int
	remoteIngressSyncSetsSuccessCount           int
	identityProviderSyncSetsSuccessCount        int
	kubeconfigSecretsSuccessCount               int
	kubeAdminCredsSecretsSuccessCount           int
	syncSetInstancesSuccessCount                int
	selectorSyncSetInstancesSuccessCount        int

	clusterProvisionsFailureCount               int
	installLogPVCsFailureCount                  int
	imagesetJobsFailureCount                    int
	clusterDeprovisionsFailureCount             int
	dnsZonesFailureCount                        int
	mergedPullSecretsFailureCount               int
	clusterDeprovisionJobsFailureCount          int
	clusterProvisionJobsFailureCount            int
	clusterStatesFailureCount                   int
	controlPlaneCertificateSyncSetsFailureCount int
	remoteIngressSyncSetsFailureCount           int
	identityProviderSyncSetsFailureCount        int
	kubeconfigSecretsFailureCount               int
	kubeAdminCredsSecretsFailureCount           int
	syncSetInstancesFailureCount                int
	selectorSyncSetInstancesFailureCount        int
}

func (o *Options) run(client client.Client) (labelMigrator, error) {
	lm := labelMigrator{
		client: client,
		logger: o.Logger,
		dryRun: o.DryRun,
	}
	lm.succeeded = true

	if err := lm.labelClusterProvisions(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling clusterprovisions")
		lm.succeeded = false
	}

	if err := lm.labelInstallLogPVCs(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling installlog PVCs")
		lm.succeeded = false
	}

	if err := lm.labelImageSetJobs(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling imageset jobs")
		lm.succeeded = false
	}

	if err := lm.labelClusterDeprovisionsRequests(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling clusterdeprovisionsrequests")
		lm.succeeded = false
	}

	if err := lm.labelDNSZones(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling dnszones")
		lm.succeeded = false
	}

	if err := lm.labelMergedPullSecrets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling mergedpullsecrets")
		lm.succeeded = false
	}

	if err := lm.labelClusterDeprovisionJobs(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling clusterdeprovision jobs")
		lm.succeeded = false
	}

	if err := lm.labelClusterProvisionJobs(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling provision jobs")
		lm.succeeded = false
	}

	if err := lm.labelClusterStates(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling clusterstates")
		lm.succeeded = false
	}

	if err := lm.labelControlPlaneCertificateSyncSets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling controlplanecertificate syncsets")
		lm.succeeded = false
	}

	if err := lm.labelRemoteIngressSyncSets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling remoteingress syncsets")
		lm.succeeded = false
	}

	if err := lm.labelIdentityProviderSyncSets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling identityprovider syncsets")
		lm.succeeded = false
	}

	if err := lm.labelKubeconfigSecrets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling kubeconfig secrets")
		lm.succeeded = false
	}

	if err := lm.labelKubeAdminCredsSecrets(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling kubeadmin secrets")
		lm.succeeded = false
	}

	if err := lm.labelSyncSetInstances(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling syncsetinstances")
		lm.succeeded = false
	}

	if err := lm.labelSelectorSyncSetInstances(); err != nil {
		o.Logger.WithError(err).Error("Failed labeling selectorsyncsetinstances")
		lm.succeeded = false
	}

	// Only display results when it's not a practice run.
	if !lm.dryRun {
		lm.logger.WithFields(logger.Fields{
			"clusterProvisionsSuccessCount":               lm.clusterProvisionsSuccessCount,
			"installLogPVCsSuccessCount":                  lm.installLogPVCsSuccessCount,
			"imagesetJobsSuccessCount":                    lm.imagesetJobsSuccessCount,
			"clusterDeprovisionsSuccessCount":             lm.clusterDeprovisionsSuccessCount,
			"dnsZonesSuccessCount":                        lm.dnsZonesSuccessCount,
			"mergedPullSecretsSuccessCount":               lm.mergedPullSecretsSuccessCount,
			"clusterDeprovisionJobsSuccessCount":          lm.clusterDeprovisionJobsSuccessCount,
			"clusterProvisionJobsSuccessCount":            lm.clusterProvisionJobsSuccessCount,
			"clusterStatesSuccessCount":                   lm.clusterStatesSuccessCount,
			"controlPlaneCertificateSyncSetsSuccessCount": lm.controlPlaneCertificateSyncSetsSuccessCount,
			"remoteIngressSyncSetsSuccessCount":           lm.remoteIngressSyncSetsSuccessCount,
			"identityProviderSyncSetsSuccessCount":        lm.identityProviderSyncSetsSuccessCount,
			"kubeconfigSecretsSuccessCount":               lm.kubeconfigSecretsSuccessCount,
			"kubeAdminCredsSecretsSuccessCount":           lm.kubeAdminCredsSecretsSuccessCount,
			"syncSetInstancesSuccessCount":                lm.syncSetInstancesSuccessCount,
			"selectorSyncSetInstancesSuccessCount":        lm.selectorSyncSetInstancesSuccessCount,
		}).Info("Successful Object Labelings")

		lm.logger.WithFields(logger.Fields{
			"clusterProvisionsFailureCount":               lm.clusterProvisionsFailureCount,
			"installLogPVCsFailureCount":                  lm.installLogPVCsFailureCount,
			"imagesetJobsFailureCount":                    lm.imagesetJobsFailureCount,
			"clusterDeprovisionsFailureCount":             lm.clusterDeprovisionsFailureCount,
			"dnsZonesFailureCount":                        lm.dnsZonesFailureCount,
			"mergedPullSecretsFailureCount":               lm.mergedPullSecretsFailureCount,
			"clusterDeprovisionJobsFailureCount":          lm.clusterDeprovisionJobsFailureCount,
			"clusterProvisionJobsFailureCount":            lm.clusterProvisionJobsFailureCount,
			"clusterStatesFailureCount":                   lm.clusterStatesFailureCount,
			"controlPlaneCertificateSyncSetsFailureCount": lm.controlPlaneCertificateSyncSetsFailureCount,
			"remoteIngressSyncSetsFailureCount":           lm.remoteIngressSyncSetsFailureCount,
			"identityProviderSyncSetsFailureCount":        lm.identityProviderSyncSetsFailureCount,
			"kubeconfigSecretsFailureCount":               lm.kubeconfigSecretsFailureCount,
			"kubeAdminCredsSecretsFailureCount":           lm.kubeAdminCredsSecretsFailureCount,
			"syncSetInstancesFailureCount":                lm.syncSetInstancesFailureCount,
			"selectorSyncSetInstancesFailureCount":        lm.selectorSyncSetInstancesFailureCount,
		}).Info("Failed Object Labelings")
	}

	if !lm.succeeded {
		return lm, errors.New("Label migration failed")
	}

	return lm, nil
}

func (l *labelMigrator) labelClusterProvisions() error {
	all := &hivev1alpha1.ClusterProvisionList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.clusterProvisionsFailureCount++
		return errors.Wrap(err, "Failed listing ClusterProvisions")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.clusterProvisionsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.clusterProvisionsSuccessCount++
		} else {
			l.clusterProvisionsFailureCount++
		}
	}

	if l.clusterProvisionsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.clusterProvisionsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelInstallLogPVCs() error {
	all := &corev1.PersistentVolumeClaimList{}
	labelSelector := map[string]string{constants.InstallJobLabel: "true"}
	err := l.client.List(context.TODO(), all, client.MatchingLabels(labelSelector))
	if err != nil {
		l.installLogPVCsFailureCount++
		return errors.Wrap(err, "Failed listing PersistentVolumeClaims")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "PersistentVolumeClaim")

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.installLogPVCsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.PVCTypeLabel, constants.PVCTypeInstallLogs)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.installLogPVCsSuccessCount++
		} else {
			l.installLogPVCsFailureCount++
		}
	}

	if l.installLogPVCsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.installLogPVCsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelImageSetJobs() error {
	all := &batchv1.JobList{}
	labelSelector := map[string]string{hiveimageset.ImagesetJobLabel: "true"}
	err := l.client.List(context.TODO(), all, client.MatchingLabels(labelSelector))
	if err != nil {
		l.imagesetJobsFailureCount++
		return errors.Wrap(err, "Failed listing Jobs")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Job")

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.imagesetJobsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.JobTypeLabel, constants.JobTypeImageSet)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.imagesetJobsSuccessCount++
		} else {
			l.imagesetJobsFailureCount++
		}
	}

	if l.imagesetJobsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.imagesetJobsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelClusterDeprovisionsRequests() error {
	all := &hivev1alpha1.ClusterDeprovisionRequestList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.clusterDeprovisionsFailureCount++
		return errors.Wrap(err, "Failed listing ClusterDeprovisionRequests")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.clusterDeprovisionsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.clusterDeprovisionsSuccessCount++
		} else {
			l.clusterDeprovisionsFailureCount++
		}
	}

	if l.clusterDeprovisionsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.clusterDeprovisionsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelDNSZones() error {
	all := &hivev1alpha1.DNSZoneList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.dnsZonesFailureCount++
		return errors.Wrap(err, "Failed listing DNSZones")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.dnsZonesFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.dnsZonesSuccessCount++
		} else {
			l.dnsZonesFailureCount++
		}
	}

	if l.dnsZonesFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.dnsZonesFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelMergedPullSecrets() error {
	all := &corev1.SecretList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.mergedPullSecretsFailureCount++
		return errors.Wrap(err, "Failed listing Secrets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Secret")

		if item.Type != corev1.SecretTypeDockerConfigJson {
			// Not a pull secret, wrong type
			continue
		}

		if _, ok := item.Data[corev1.DockerConfigJsonKey]; !ok {
			// Not a pull secret, missing key
			continue
		}

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SecretTypeLabel, constants.SecretTypeMergedPullSecret)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.mergedPullSecretsSuccessCount++
		} else {
			l.mergedPullSecretsFailureCount++
		}
	}

	if l.mergedPullSecretsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.mergedPullSecretsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelClusterDeprovisionJobs() error {
	all := &batchv1.JobList{}
	labelSelector := map[string]string{constants.UninstallJobLabel: "true"}
	err := l.client.List(context.TODO(), all, client.MatchingLabels(labelSelector))
	if err != nil {
		l.clusterDeprovisionJobsFailureCount++
		return errors.Wrap(err, "Failed listing Jobs")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Job")
		name, found := findControllerOwnerNameByKind(&item, "ClusterDeprovisionRequest")
		if !found {
			objLogger.Error("Missing ClusterDeprovisionRequest owner reference")
			l.clusterDeprovisionJobsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeprovisionNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.JobTypeLabel, constants.JobTypeDeprovision)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.clusterDeprovisionJobsSuccessCount++
		} else {
			l.clusterDeprovisionJobsFailureCount++
		}
	}

	if l.clusterDeprovisionJobsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.clusterDeprovisionJobsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelClusterProvisionJobs() error {
	all := &batchv1.JobList{}
	labelSelector := map[string]string{constants.InstallJobLabel: "true"}
	err := l.client.List(context.TODO(), all, client.MatchingLabels(labelSelector))
	if err != nil {
		l.clusterProvisionJobsFailureCount++
		return errors.Wrap(err, "Failed listing Jobs")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Job")

		name, found := findControllerOwnerNameByKind(&item, "ClusterProvision")
		if !found {
			objLogger.Error("Missing ClusterProvision owner reference")
			l.clusterProvisionJobsFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterProvisionNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.JobTypeLabel, constants.JobTypeProvision)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.clusterProvisionJobsSuccessCount++
		} else {
			l.clusterProvisionJobsFailureCount++
		}
	}

	if l.clusterProvisionJobsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.clusterProvisionJobsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelClusterStates() error {
	all := &hivev1alpha1.ClusterStateList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.clusterStatesFailureCount++
		return errors.Wrap(err, "Failed listing ClusterStates")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.clusterStatesFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.clusterStatesSuccessCount++
		} else {
			l.clusterStatesFailureCount++
		}
	}

	if l.clusterStatesFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.clusterStatesFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelControlPlaneCertificateSyncSets() error {
	all := &hivev1alpha1.SyncSetList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.controlPlaneCertificateSyncSetsFailureCount++
		return errors.Wrap(err, "Failed listing SyncSets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			continue
		}

		if !isControlPlaneCertsSyncSet(item, name) {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeControlPlaneCerts)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.controlPlaneCertificateSyncSetsSuccessCount++
		} else {
			l.controlPlaneCertificateSyncSetsFailureCount++
		}
	}

	if l.controlPlaneCertificateSyncSetsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.controlPlaneCertificateSyncSetsFailureCount)
	}

	return nil
}

func isControlPlaneCertsSyncSet(syncset hivev1alpha1.SyncSet, clusterDeploymentName string) bool {
	return hivecontrolplanecerts.GenerateControlPlaneCertsSyncSetName(clusterDeploymentName) == syncset.Name
}

func (l *labelMigrator) labelRemoteIngressSyncSets() error {
	all := &hivev1alpha1.SyncSetList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.remoteIngressSyncSetsFailureCount++
		return errors.Wrap(err, "Failed listing SyncSets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			continue
		}

		if !isRemoteIngressSyncSet(item, name) {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeRemoteIngress)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.remoteIngressSyncSetsSuccessCount++
		} else {
			l.remoteIngressSyncSetsFailureCount++
		}
	}

	if l.remoteIngressSyncSetsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.remoteIngressSyncSetsFailureCount)
	}

	return nil
}

func isRemoteIngressSyncSet(syncset hivev1alpha1.SyncSet, clusterDeploymentName string) bool {
	return hiveremoteingress.GenerateRemoteIngressSyncSetName(clusterDeploymentName) == syncset.Name
}

func (l *labelMigrator) labelIdentityProviderSyncSets() error {
	all := &hivev1alpha1.SyncSetList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.identityProviderSyncSetsFailureCount++
		return errors.Wrap(err, "Failed listing SyncSets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			continue
		}

		if !isIdentityProviderSyncSet(item, name) {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeIdentityProvider)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.identityProviderSyncSetsSuccessCount++
		} else {
			l.identityProviderSyncSetsFailureCount++
		}
	}

	if l.identityProviderSyncSetsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.identityProviderSyncSetsFailureCount)
	}

	return nil
}

func isIdentityProviderSyncSet(syncset hivev1alpha1.SyncSet, clusterDeploymentName string) bool {
	return hivesyncidentityprovider.GenerateIdentityProviderSyncSetName(clusterDeploymentName) == syncset.Name
}

func (l *labelMigrator) labelKubeconfigSecrets() error {
	all := &corev1.SecretList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.kubeconfigSecretsFailureCount++
		return errors.Wrap(err, "Failed listing Secrets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Secret")

		if _, ok := item.Data[constants.KubeconfigSecretKey]; !ok {
			// Not a kubeconfig secret, missing key
			continue
		}

		name, found := findControllerOwnerNameByKind(&item, "ClusterProvision")
		if !found {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterProvisionNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeConfig)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.kubeconfigSecretsSuccessCount++
		} else {
			l.kubeconfigSecretsFailureCount++
		}
	}

	if l.kubeconfigSecretsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.kubeconfigSecretsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelKubeAdminCredsSecrets() error {
	all := &corev1.SecretList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.kubeAdminCredsSecretsFailureCount++
		return errors.Wrap(err, "Failed listing Secrets")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, "Secret")

		if _, ok := item.Data[constants.UsernameSecretKey]; !ok {
			// Not an admin credentials secret, missing key
			continue
		}

		if _, ok := item.Data[constants.PasswordSecretKey]; !ok {
			// Not an admin credentials secret, missing key
			continue
		}

		name, found := findControllerOwnerNameByKind(&item, "ClusterProvision")
		if !found {
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterProvisionNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeAdminCreds)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.kubeAdminCredsSecretsSuccessCount++
		} else {
			l.kubeAdminCredsSecretsFailureCount++
		}
	}

	if l.kubeAdminCredsSecretsFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.kubeAdminCredsSecretsFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelSyncSetInstances() error {
	all := &hivev1alpha1.SyncSetInstanceList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.syncSetInstancesFailureCount++
		return errors.Wrap(err, "Failed listing SyncSetInstances")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		if item.Spec.SyncSet == nil && item.Spec.SelectorSyncSet == nil {
			objLogger.Error("Missing both .Spec.SyncSet and .Spec.SelectorSyncSet. One of these should be set")
			l.syncSetInstancesFailureCount++
			continue
		}

		if item.Spec.SyncSet == nil {
			// Not created from a syncset.
			continue
		}

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.syncSetInstancesFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SyncSetNameLabel, item.Spec.SyncSet.Name)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.syncSetInstancesSuccessCount++
		} else {
			l.syncSetInstancesFailureCount++
		}
	}

	if l.syncSetInstancesFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.syncSetInstancesFailureCount)
	}

	return nil
}

func (l *labelMigrator) labelSelectorSyncSetInstances() error {
	all := &hivev1alpha1.SyncSetInstanceList{}

	err := l.client.List(context.TODO(), all)
	if err != nil {
		l.selectorSyncSetInstancesFailureCount++
		return errors.Wrap(err, "Failed listing SyncSetInstances")
	}

	for _, item := range all.Items {
		objLogger := l.newObjectLogger(item.Name, item.Namespace, item.Kind)

		if item.Spec.SyncSet == nil && item.Spec.SelectorSyncSet == nil {
			objLogger.Error("Missing both .Spec.SyncSet and .Spec.SelectorSyncSet. One of these should be set")
			l.selectorSyncSetInstancesFailureCount++
			continue
		}

		if item.Spec.SelectorSyncSet == nil {
			// Not created from a selectorsyncset.
			continue
		}

		name, found := findControllerOwnerNameByKind(&item, "ClusterDeployment")
		if !found {
			objLogger.Error("Missing ClusterDeployment owner reference")
			l.selectorSyncSetInstancesFailureCount++
			continue
		}

		item.Labels = k8slabels.AddLabel(item.Labels, constants.ClusterDeploymentNameLabel, name)
		item.Labels = k8slabels.AddLabel(item.Labels, constants.SelectorSyncSetNameLabel, item.Spec.SelectorSyncSet.Name)

		if l.updateObject(objLogger.WithField("labels", item.Labels), &item) {
			l.selectorSyncSetInstancesSuccessCount++
		} else {
			l.selectorSyncSetInstancesFailureCount++
		}
	}

	if l.selectorSyncSetInstancesFailureCount > 0 {
		return fmt.Errorf("Failed updating %v object(s)", l.selectorSyncSetInstancesFailureCount)
	}

	return nil
}

func findControllerOwnerNameByKind(object metav1.Object, kind string) (string, bool) {
	for _, ownerRef := range object.GetOwnerReferences() {
		if *ownerRef.Controller && ownerRef.Kind == kind {
			// FOUND IT
			return ownerRef.Name, true
		}
	}

	return "", false
}

func (l *labelMigrator) updateObject(objLogger logger.FieldLogger, object runtime.Object) bool {
	if l.dryRun {
		objLogger.Info("Dry Run: Would have written migrated object to cluster")
		return true
	}

	if err := l.client.Update(context.TODO(), object); err != nil {
		objLogger.WithError(err).Error("Failed updating migrated object in cluster.")
		return false
	}

	objLogger.Info("Succeeded updating migrated object in cluster")
	return true
}

func (l *labelMigrator) newObjectLogger(name, namespace, kind string) logger.FieldLogger {
	return l.logger.WithFields(logger.Fields{
		"name":      name,
		"namespace": namespace,
		"kind":      kind,
	})
}
