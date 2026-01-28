package machinepool

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	cpms "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/util/logrus"
	"github.com/openshift/hive/pkg/util/scheme"
)

// MachineSetAnalysis contains the analyzed data from MachineSets for adoption.
type MachineSetAnalysis struct {
	Input    AnalysisInput
	Summary  AnalysisSummary
	Platform PlatformData
	Config   ConfigData
}

type AnalysisInput struct {
	MachineSets       []machineapi.MachineSet
	ClusterDeployment *hivev1.ClusterDeployment
	Infrastructure    *configv1.Infrastructure
	RemoteClient      client.Client
}

type AnalysisSummary struct {
	Platform          string
	InstanceType      string
	Zones             []string
	TotalReplicas     int32
	ReplicasByZone    map[string]int32
	ZonesByMachineSet map[string]string
}

type PlatformData struct {
	SubnetsByZone map[string]string
	ComputeSubnet string
}

type ConfigData struct {
	ProviderConfig cpms.ProviderConfig
}

type MachineSetAnalyzer struct {
	hubClient       client.Client
	cd              *hivev1.ClusterDeployment
	machineSetNames []string
	poolName        string
	log             log.FieldLogger
}

type machineSetManagementStatus struct {
	name         string
	existingPool string
	poolExists   bool
}

type AdoptOptions struct {
	ClusterDeploymentName string
	Namespace             string
	MachineSets           []string
	PoolName              string
	Output                string
	DryRun                bool
	log                   log.FieldLogger
	hubClient             client.Client
	clusterDeploy         *hivev1.ClusterDeployment
}

// NewAdoptCommand creates the cobra command for adopting MachineSets.
func NewAdoptCommand() *cobra.Command {
	opt := &AdoptOptions{log: log.WithField("command", "machinepool adopt")}

	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt existing MachineSets into Hive MachinePool management",
		Long: `Adopt existing MachineSets into Hive MachinePool management.

This command follows the documented adoption procedure:
1. Pauses ClusterDeployment reconciliation
2. Labels MachineSets in the spoke cluster
3. Creates MachinePool in the hub cluster
4. Resumes ClusterDeployment reconciliation
`,
		Example: `  # Preview adoption plan (dry-run)
  hiveutil machinepool adopt -c mycluster -n mynamespace --machine-sets ms1,ms2 --pool worker --dry-run

  # Preview adoption plan (output format)
  hiveutil machinepool adopt -c mycluster -n mynamespace --machine-sets ms1,ms2 --pool worker --output yaml

  # Apply adoption
  hiveutil machinepool adopt -c mycluster -n mynamespace --machine-sets ms1,ms2 --pool worker`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(); err != nil {
				opt.log.WithError(err).Fatal("Failed to complete options")
			}
			if err := opt.Validate(); err != nil {
				opt.log.WithError(err).Fatal("Invalid options")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Failed to adopt machinepool")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opt.ClusterDeploymentName, "cluster-deployment", "c", "", "ClusterDeployment name (required)")
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace (required)")
	flags.StringSliceVar(&opt.MachineSets, "machine-sets", []string{}, "Comma-separated list of MachineSet names to adopt, can be specified multiple times (required)")
	flags.StringVar(&opt.PoolName, "pool", "", "MachinePool name (e.g., worker, infra) (required)")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output format (yaml|json). If not specified, resources will be applied to cluster")
	flags.BoolVar(&opt.DryRun, "dry-run", false, "Show what will be done without making changes (same as --output yaml)")

	cmd.MarkFlagRequired("cluster-deployment")
	cmd.MarkFlagRequired("namespace")
	cmd.MarkFlagRequired("machine-sets")
	cmd.MarkFlagRequired("pool")

	return cmd
}

// Complete initializes the AdoptOptions with required clients.
func (o *AdoptOptions) Complete() error {
	client, err := utils.GetClient("machinepool-adopt")
	if err != nil {
		return err
	}
	o.hubClient = client

	o.clusterDeploy = &hivev1.ClusterDeployment{}
	key := types.NamespacedName{Namespace: o.Namespace, Name: o.ClusterDeploymentName}
	if err := o.hubClient.Get(context.Background(), key, o.clusterDeploy); err != nil {
		return errors.Wrapf(err, "failed to get ClusterDeployment %s/%s", o.Namespace, o.ClusterDeploymentName)
	}
	return nil
}

// Validate validates all AdoptOptions fields.
func (o *AdoptOptions) Validate() error {
	o.MachineSets = o.normalizeMachineSets()
	if o.DryRun && o.Output == "" {
		o.Output = "yaml"
	}

	var errs []string
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		errs = append(errs, fmt.Sprintf("invalid output format: %s. Valid formats: yaml, json", o.Output))
	}
	if len(o.MachineSets) == 0 {
		errs = append(errs, "--machine-sets is required and must specify at least one MachineSet")
	}
	if o.clusterDeploy == nil {
		errs = append(errs, "ClusterDeployment not loaded, Complete() must be called first")
	} else if !o.clusterDeploy.Spec.Installed {
		errs = append(errs, fmt.Sprintf("ClusterDeployment %s/%s is not installed yet", o.Namespace, o.ClusterDeploymentName))
	} else if o.clusterDeploy.DeletionTimestamp != nil {
		errs = append(errs, fmt.Sprintf("ClusterDeployment %s/%s is being deleted", o.Namespace, o.ClusterDeploymentName))
	}

	mpName := GetMachinePoolName(o.ClusterDeploymentName, o.PoolName)
	existingMP := &hivev1.MachinePool{}
	key := types.NamespacedName{Namespace: o.Namespace, Name: mpName}
	if err := o.hubClient.Get(context.Background(), key, existingMP); err == nil {
		errs = append(errs, fmt.Sprintf("MachinePool %s/%s already exists", o.Namespace, mpName))
	} else if !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Sprintf("failed to check if MachinePool %s/%s exists: %v", o.Namespace, mpName, err))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Run executes the adopt command.
func (o *AdoptOptions) Run() error {
	objs, analysis, err := o.GenerateObjects()
	if err != nil {
		return err
	}

	machinePool, ok := objs[0].(*hivev1.MachinePool)
	if !ok {
		return errors.New("unexpected object type in GenerateObjects result")
	}

	if o.Output != "" || o.DryRun {
		o.printAdoptionPlan(analysis, machinePool)
		if o.Output != "" {
			PrintObjects(objs, scheme.GetScheme(), CreatePrinter(o.Output))
		}
		return nil
	}
	return o.runApply(context.Background(), analysis, machinePool)
}

// GenerateObjects generates the MachinePool objects to be created.
func (o *AdoptOptions) GenerateObjects() ([]runtime.Object, *MachineSetAnalysis, error) {
	if o.clusterDeploy == nil {
		return nil, nil, errors.New("ClusterDeployment not loaded, Complete() must be called first")
	}

	analyzer := NewMachineSetAnalyzer(o.hubClient, o.clusterDeploy, o.MachineSets, o.PoolName, o.log)
	o.log.WithField("count", len(o.MachineSets)).Info("Analyzing MachineSets in spoke cluster")
	analysis, err := analyzer.Analyze(context.Background())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to analyze MachineSets")
	}
	o.log.WithFields(log.Fields{
		"replicas": analysis.Summary.TotalReplicas,
		"zones":    len(analysis.Summary.Zones),
	}).Info("Analysis complete")

	builder, err := NewMachinePoolBuilder(o.clusterDeploy, o.PoolName, analysis, o.log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create MachinePool builder")
	}
	machinePool, err := builder.Build()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to build MachinePool")
	}
	return []runtime.Object{machinePool}, analysis, nil
}

// printAdoptionPlan prints the adoption plan for dry-run mode.
func (o *AdoptOptions) printAdoptionPlan(analysis *MachineSetAnalysis, machinePool *hivev1.MachinePool) {
	mode := "Preview"
	if o.DryRun {
		mode = "Dry-run"
	}
	mpName := fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name)
	o.log.WithFields(log.Fields{
		"mode": mode, "cluster": o.ClusterDeploymentName, "pool": o.PoolName,
		"machinepool": mpName, "machinesets": len(analysis.Input.MachineSets),
		"platform": analysis.Summary.Platform, "instance": analysis.Summary.InstanceType,
		"replicas": analysis.Summary.TotalReplicas, "zones": len(analysis.Summary.Zones),
	}).Info("MachinePool Adoption Plan")

	o.log.Info("MachineSets to adopt:")
	for i, ms := range analysis.Input.MachineSets {
		zone := analysis.Summary.ZonesByMachineSet[ms.Name]
		if zone == "" {
			zone = "<none>"
		}
		replicas := int32(0)
		if ms.Spec.Replicas != nil {
			replicas = *ms.Spec.Replicas
		}
		o.log.WithFields(log.Fields{
			"index": i + 1, "machineset": ms.Name, "replicas": replicas,
			"zone": zone, "type": analysis.Summary.InstanceType,
		}).Info("  MachineSet")
	}

	o.log.WithFields(log.Fields{
		"pool_label":    fmt.Sprintf("%s=%s", machinePoolNameLabel, o.PoolName),
		"managed_label": fmt.Sprintf("%s=true", constants.HiveManagedLabel),
	}).Info("Labels to be applied")

	if len(analysis.Summary.Zones) <= 1 {
		return
	}
	o.log.Info("Zone distribution:")
	for i, zone := range analysis.Summary.Zones {
		o.log.WithFields(log.Fields{
			"order": i + 1, "zone": zone, "replicas": analysis.Summary.ReplicasByZone[zone],
		}).Info("  Zone")
	}
}

// runApply executes the adoption procedure with reconciliation pause.
func (o *AdoptOptions) runApply(ctx context.Context, analysis *MachineSetAnalysis, machinePool *hivev1.MachinePool) error {
	mpName := fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name)
	o.log.WithFields(log.Fields{
		"cluster": o.ClusterDeploymentName, "pool": o.PoolName, "machinepool": mpName,
		"machinesets": len(o.MachineSets), "replicas": analysis.Summary.TotalReplicas,
	}).Info("Starting MachinePool adoption")

	originalLabels := make(map[string]map[string]string, len(o.MachineSets))
	labelsApplied := false
	if err := o.withReconciliationPause(ctx, func() error {
		defer func() {
			if labelsApplied && len(originalLabels) > 0 {
				o.log.Info("Rolling back MachineSet labels due to adoption failure")
				if err := o.rollbackLabels(ctx, analysis, originalLabels); err != nil {
					o.log.WithError(err).Error("Failed to rollback labels - manual cleanup may be required")
					for msName := range originalLabels {
						o.log.WithField("machineset", msName).Warn("MachineSet may have orphaned Hive labels and needs manual cleanup")
					}
					o.log.Warnf("To manually remove labels, run: oc label machineset -n %s <machineset-name> %s- %s-",
						machineAPINamespace, machinePoolNameLabel, constants.HiveManagedLabel)
				} else {
					o.log.Info("Labels rolled back successfully")
				}
			}
		}()

		if err := o.labelMachineSets(ctx, analysis, originalLabels); err != nil {
			return err
		}
		labelsApplied = true
		return o.createMachinePool(ctx, machinePool)
	}); err != nil {
		return err
	}

	o.log.WithFields(log.Fields{"cluster": o.ClusterDeploymentName, "pool": o.PoolName, "machinepool": mpName}).
		Info("MachinePool adoption completed successfully")
	return nil
}

// withReconciliationPause pauses reconciliation, executes fn, then resumes reconciliation.
func (o *AdoptOptions) withReconciliationPause(ctx context.Context, fn func() error) error {
	cdName := fmt.Sprintf("%s/%s", o.Namespace, o.ClusterDeploymentName)
	o.log.WithField("clusterdeployment", cdName).Info("Step 1/3: Pausing reconciliation")
	if err := PauseReconciliation(ctx, o.hubClient, o.clusterDeploy); err != nil {
		return errors.Wrap(err, "failed to pause reconciliation")
	}
	o.log.Info("Reconciliation paused successfully")

	defer func() {
		var panicked interface{}
		if r := recover(); r != nil {
			panicked = r
			o.log.WithField("panic", r).Error("Panic occurred during adoption, attempting to resume reconciliation")
		}
		o.log.WithField("clusterdeployment", cdName).Info("Step 3/3: Resuming reconciliation")
		if err := ResumeReconciliation(ctx, o.hubClient, o.clusterDeploy); err != nil {
			o.log.WithError(err).Error("Failed to resume reconciliation")
			o.log.Warnf("Manually remove annotation: oc annotate clusterdeployment %s -n %s %s-",
				o.ClusterDeploymentName, o.Namespace, constants.ReconcilePauseAnnotation)
		} else {
			o.log.Info("Reconciliation resumed successfully")
		}
		if panicked != nil {
			panic(panicked)
		}
	}()

	o.log.Info("Step 2/3: Applying changes")
	return fn()
}

// labelMachineSets labels MachineSets with Hive management labels.
func (o *AdoptOptions) labelMachineSets(ctx context.Context, analysis *MachineSetAnalysis, originalLabels map[string]map[string]string) error {
	o.log.WithField("count", len(o.MachineSets)).Info("Step 2a/3: Labeling MachineSets in spoke cluster")
	for i, msName := range o.MachineSets {
		o.log.WithFields(log.Fields{"machineset": msName, "progress": fmt.Sprintf("%d/%d", i+1, len(o.MachineSets))}).
			Info("  Applying labels to MachineSet")
		if _, exists := originalLabels[msName]; !exists {
			if backup := o.backupMachineSetLabels(ctx, analysis, msName); backup != nil {
				originalLabels[msName] = backup
			} else {
				o.log.Warnf("Failed to backup labels for MachineSet %s, rollback may be incomplete", msName)
			}
		}
		if err := UpdateMachineSetLabels(ctx, analysis.Input.RemoteClient, msName, func(ms *machineapi.MachineSet) {
			AddMachinePoolLabels(ms, o.PoolName)
		}); err != nil {
			return errors.Wrapf(err, "failed to label MachineSet %s in namespace %s", msName, machineAPINamespace)
		}
		o.log.WithField("machineset", msName).Info("  MachineSet labeled successfully")
	}
	o.log.WithField("count", len(o.MachineSets)).Info("All MachineSets labeled successfully")
	return nil
}

// createMachinePool creates the MachinePool resource in the hub cluster.
func (o *AdoptOptions) createMachinePool(ctx context.Context, machinePool *hivev1.MachinePool) error {
	mpName := fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name)
	o.log.WithField("machinepool", mpName).Info("Step 2b/3: Creating MachinePool in hub cluster")
	if err := o.hubClient.Create(ctx, machinePool); err != nil {
		return errors.Wrapf(err, "failed to create MachinePool %s in hub cluster", mpName)
	}
	o.log.WithField("machinepool", mpName).Info("MachinePool created successfully")
	return nil
}

// backupMachineSetLabels backs up the current labels of a MachineSet.
func (o *AdoptOptions) backupMachineSetLabels(ctx context.Context, analysis *MachineSetAnalysis, msName string) map[string]string {
	ms := &machineapi.MachineSet{}
	key := client.ObjectKey{Namespace: machineAPINamespace, Name: msName}
	if err := analysis.Input.RemoteClient.Get(ctx, key, ms); err != nil {
		o.log.WithError(err).Warnf("Failed to fetch MachineSet %s for label backup", msName)
		return nil
	}
	if len(ms.Labels) == 0 {
		return nil
	}
	backup := make(map[string]string, len(ms.Labels))
	maps.Copy(backup, ms.Labels)
	return backup
}

// rollbackLabels rolls back MachineSet labels to their original state.
func (o *AdoptOptions) rollbackLabels(ctx context.Context, analysis *MachineSetAnalysis, originalLabels map[string]map[string]string) error {
	var rollbackErrors []string
	for msName, original := range originalLabels {
		if err := UpdateMachineSetLabels(ctx, analysis.Input.RemoteClient, msName, func(ms *machineapi.MachineSet) {
			RestoreMachineSetLabels(ms, original)
		}); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("MachineSet %s: %v", msName, err))
			o.log.WithError(err).Errorf("Failed to rollback labels for MachineSet %s", msName)
		}
	}
	if len(rollbackErrors) > 0 {
		o.log.WithFields(log.Fields{
			"successful": len(originalLabels) - len(rollbackErrors),
			"failed":     len(rollbackErrors), "total": len(originalLabels),
		}).Error("Partial rollback failure - some MachineSets may have orphaned labels")
		return errors.Errorf("failed to rollback labels for %d MachineSet(s): %s", len(rollbackErrors), strings.Join(rollbackErrors, "; "))
	}
	o.log.WithField("count", len(originalLabels)).Info("All MachineSet labels rolled back successfully")
	return nil
}

// MachineSetAnalyzer methods

// NewMachineSetAnalyzer creates a new MachineSetAnalyzer.
func NewMachineSetAnalyzer(hubClient client.Client, cd *hivev1.ClusterDeployment, machineSetNames []string, poolName string, logger log.FieldLogger) *MachineSetAnalyzer {
	return &MachineSetAnalyzer{
		hubClient:       hubClient,
		cd:              cd,
		machineSetNames: machineSetNames,
		poolName:        poolName,
		log:             logger,
	}
}

// Analyze analyzes MachineSets and returns a MachineSetAnalysis.
func (a *MachineSetAnalyzer) Analyze(ctx context.Context) (*MachineSetAnalysis, error) {
	remoteClient, err := remoteclient.NewBuilder(a.hubClient, a.cd, "machinepool-adopt").Build()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create remote client")
	}

	machineSets, notFound, err := a.fetchMachineSets(ctx, remoteClient)
	if err != nil {
		return nil, err
	}
	if len(notFound) > 0 {
		return nil, errors.Errorf("MachineSet(s) not found in namespace %s: %v", machineAPINamespace, notFound)
	}
	if len(machineSets) == 0 {
		return nil, errors.New("no MachineSets found - verify MachineSet names are correct")
	}

	alreadyManaged, err := a.validateMachineSetManagement(ctx, machineSets)
	if err != nil {
		return nil, err
	}
	if len(alreadyManaged) > 0 {
		var msg strings.Builder
		msg.WriteString("Cannot adopt MachineSet(s) that are already managed:\n")
		for _, s := range alreadyManaged {
			mpName := GetMachinePoolName(a.cd.Name, s.existingPool)
			if s.poolExists {
				fmt.Fprintf(&msg, "  - %s: already managed by MachinePool %s/%s\n", s.name, a.cd.Namespace, mpName)
			} else {
				fmt.Fprintf(&msg, "  - %s: has machine-pool label '%s' but MachinePool %s/%s doesn't exist (orphaned label)\n", s.name, s.existingPool, a.cd.Namespace, mpName)
			}
		}
		msg.WriteString("\nTo adopt these MachineSets, first detach them from their current MachinePool or remove the orphaned labels.")
		return nil, errors.New(msg.String())
	}

	platform := GetPlatformFromClusterDeployment(a.cd)
	if platform == constants.PlatformUnknown {
		return nil, errors.Errorf("ClusterDeployment %s/%s has no platform configuration", a.cd.Namespace, a.cd.Name)
	}
	handler, err := GetPlatformHandler(platform)
	if err != nil {
		return nil, errors.Wrapf(err, "unsupported platform: %s", platform)
	}

	infrastructure, err := a.getInfrastructure(ctx, remoteClient)
	if err != nil {
		a.log.WithError(err).Warn("Failed to get Infrastructure resource, continuing without it")
	}

	analysis := &MachineSetAnalysis{
		Input: AnalysisInput{
			MachineSets:       machineSets,
			ClusterDeployment: a.cd,
			Infrastructure:    infrastructure,
			RemoteClient:      remoteClient,
		},
		Summary: AnalysisSummary{
			Platform:          platform,
			ReplicasByZone:    make(map[string]int32),
			ZonesByMachineSet: make(map[string]string),
		},
		Platform: PlatformData{
			SubnetsByZone: make(map[string]string),
		},
	}

	logr := logrus.NewLogr(a.log)
	instanceTypes := make(map[string]bool, len(machineSets))
	var firstProviderConfig cpms.ProviderConfig

	for i := range analysis.Input.MachineSets {
		ms := &analysis.Input.MachineSets[i]
		providerConfig, err := cpms.NewProviderConfigFromMachineSpec(logr, ms.Spec.Template.Spec, analysis.Input.Infrastructure)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse MachineSet %s provider config", ms.Name)
		}
		if firstProviderConfig == nil {
			firstProviderConfig = providerConfig
		}

		zone, err := handler.ExtractZone(providerConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to extract zone from MachineSet %s", ms.Name)
		}
		if err := handler.ValidateFailureDomain(zone, a.cd); err != nil {
			return nil, err
		}

		instanceType, err := handler.ExtractInstanceType(providerConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to extract instance type from MachineSet %s", ms.Name)
		}
		instanceTypes[instanceType] = true

		if ms.Spec.Replicas == nil {
			return nil, errors.Errorf("MachineSet %s has nil replicas", ms.Name)
		}
		replicas := *ms.Spec.Replicas
		analysis.Summary.TotalReplicas += replicas
		if zone != "" {
			analysis.Summary.ReplicasByZone[zone] += replicas
			analysis.Summary.ZonesByMachineSet[ms.Name] = zone
		}

		if err := handler.ExtractSubnet(providerConfig, zone, analysis); err != nil {
			a.log.WithError(err).Warnf("Failed to extract subnet from MachineSet %s", ms.Name)
		}
	}

	switch len(instanceTypes) {
	case 0:
		return nil, errors.New("no instance types found in MachineSets")
	case 1:
		for t := range instanceTypes {
			analysis.Summary.InstanceType = t
		}
	default:
		typeList := make([]string, 0, len(instanceTypes))
		for t := range instanceTypes {
			typeList = append(typeList, t)
		}
		return nil, errors.Errorf("inconsistent instance types: %v", typeList)
	}

	a.calculateZoneOrder(analysis)

	if err := handler.ExtractPlatformAggregatedData(analysis); err != nil {
		return nil, err
	}

	if firstProviderConfig != nil {
		analysis.Config.ProviderConfig = firstProviderConfig
	}

	a.log.WithFields(log.Fields{
		"platform":       analysis.Summary.Platform,
		"instanceType":   analysis.Summary.InstanceType,
		"zones":          analysis.Summary.Zones,
		"totalReplicas":  analysis.Summary.TotalReplicas,
		"replicasByZone": analysis.Summary.ReplicasByZone,
	}).Debug("MachineSet analysis completed")

	return analysis, nil
}

// fetchMachineSets fetches MachineSets from the spoke cluster.
func (a *MachineSetAnalyzer) fetchMachineSets(ctx context.Context, remoteClient client.Client) ([]machineapi.MachineSet, []string, error) {
	machineSets := make([]machineapi.MachineSet, 0, len(a.machineSetNames))
	notFound := make([]string, 0)
	for _, name := range a.machineSetNames {
		ms := &machineapi.MachineSet{}
		key := client.ObjectKey{Namespace: machineAPINamespace, Name: name}
		if err := remoteClient.Get(ctx, key, ms); err != nil {
			if client.IgnoreNotFound(err) == nil {
				notFound = append(notFound, name)
				continue
			}
			return nil, nil, errors.Wrapf(err, "failed to get MachineSet %s", name)
		}
		machineSets = append(machineSets, *ms)
	}
	return machineSets, notFound, nil
}

// validateMachineSetManagement validates that MachineSets are not already managed.
func (a *MachineSetAnalyzer) validateMachineSetManagement(ctx context.Context, machineSets []machineapi.MachineSet) ([]machineSetManagementStatus, error) {
	alreadyManaged := make([]machineSetManagementStatus, 0)
	for i := range machineSets {
		ms := &machineSets[i]
		poolLabel, hasLabel := ms.Labels[machinePoolNameLabel]
		if !hasLabel {
			continue
		}
		mpName := GetMachinePoolName(a.cd.Name, poolLabel)
		key := types.NamespacedName{Namespace: a.cd.Namespace, Name: mpName}
		existingMP := &hivev1.MachinePool{}
		err := a.hubClient.Get(ctx, key, existingMP)
		switch {
		case err == nil:
			alreadyManaged = append(alreadyManaged, machineSetManagementStatus{
				name: ms.Name, existingPool: poolLabel, poolExists: true,
			})
			a.log.Warnf("MachineSet %s already managed by MachinePool %s/%s", ms.Name, a.cd.Namespace, mpName)
		case apierrors.IsNotFound(err):
			alreadyManaged = append(alreadyManaged, machineSetManagementStatus{
				name: ms.Name, existingPool: poolLabel, poolExists: false,
			})
			a.log.Warnf("MachineSet %s has machine-pool label but MachinePool %s/%s doesn't exist", ms.Name, a.cd.Namespace, mpName)
		default:
			return nil, errors.Wrapf(err, "failed to check MachinePool %s/%s", a.cd.Namespace, mpName)
		}
	}
	return alreadyManaged, nil
}

func (a *MachineSetAnalyzer) calculateZoneOrder(analysis *MachineSetAnalysis) {
	type zoneReplica struct {
		zone     string
		replicas int32
	}
	zones := make([]zoneReplica, 0, len(analysis.Summary.ReplicasByZone))
	for zone, replicas := range analysis.Summary.ReplicasByZone {
		if zone != "" {
			zones = append(zones, zoneReplica{zone: zone, replicas: replicas})
		}
	}
	sort.Slice(zones, func(i, j int) bool {
		if zones[i].replicas != zones[j].replicas {
			return zones[i].replicas > zones[j].replicas
		}
		return zones[i].zone < zones[j].zone
	})
	analysis.Summary.Zones = make([]string, len(zones))
	for i, zr := range zones {
		analysis.Summary.Zones[i] = zr.zone
	}
}

// getInfrastructure fetches the Infrastructure resource from the spoke cluster.
func (a *MachineSetAnalyzer) getInfrastructure(ctx context.Context, remoteClient client.Client) (*configv1.Infrastructure, error) {
	infra := &configv1.Infrastructure{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(configv1.SchemeGroupVersion.WithKind("Infrastructure"))
	key := types.NamespacedName{Name: "cluster"}
	opts := &client.GetOptions{Raw: &metav1.GetOptions{TypeMeta: tm}}
	if err := remoteClient.Get(ctx, key, infra, opts); err != nil {
		a.log.WithError(err).Debug("Failed to fetch Infrastructure resource (optional)")
		return nil, err
	}
	return infra, nil
}

// Helper functions

// normalizeMachineSets flattens comma-separated MachineSet names into a single slice.
func (o *AdoptOptions) normalizeMachineSets() []string {
	var normalized []string
	seen := make(map[string]bool)
	for _, ms := range o.MachineSets {
		for _, part := range strings.Split(ms, ",") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				normalized = append(normalized, part)
				seen[part] = true
			}
		}
	}
	return normalized
}
