package machinepool

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/machinepoolresource"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/util/scheme"
	"k8s.io/utils/ptr"
)

const adoptControllerName = "hiveutil-machinepool-adopt"

// AdoptOptions holds options for machinepool adopt.
type AdoptOptions struct {
	ClusterDeploymentName string
	PoolName              string
	Namespace             string
	MachineSets           []string // required: MachineSet names to adopt
	Output                string

	log log.FieldLogger
}

// NewAdoptCommand returns the machinepool adopt subcommand.
func NewAdoptCommand() *cobra.Command {
	opt := &AdoptOptions{log: log.WithField("command", "machinepool adopt")}
	cmd := &cobra.Command{
		Use:   "adopt CLUSTER_DEPLOYMENT_NAME POOL_NAME --machinesets ms1,ms2,...",
		Short: "Adopt existing spoke MachineSets into a Hive MachinePool",
		Long: `Connects to the spoke cluster, reads the specified MachineSets, extracts platform config 
and replicas, and creates a Hive MachinePool. Automatically pauses CD before applying 
and resumes after. No manual platform flags needed.`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				opt.log.WithError(err).Fatal("Complete failed")
			}
			if err := opt.Validate(cmd); err != nil {
				opt.log.WithError(err).Fatal("Validate failed")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Run failed")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace of the ClusterDeployment (default from kubeconfig)")
	flags.StringSliceVar(&opt.MachineSets, "machinesets", nil, "MachineSet names in openshift-machine-api to adopt (required)")
	_ = cmd.MarkFlagRequired("machinesets")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output format: yaml or json (no apply)")
	return cmd
}

// Complete fills options from args and defaults.
func (o *AdoptOptions) Complete(cmd *cobra.Command, args []string) error {
	o.ClusterDeploymentName = args[0]
	o.PoolName = args[1]
	if o.Namespace == "" {
		ns, err := utils.DefaultNamespace()
		if err != nil {
			return errors.Wrap(err, "default namespace")
		}
		o.Namespace = ns
	}
	return nil
}

// Validate checks options.
func (o *AdoptOptions) Validate(cmd *cobra.Command) error {
	if len(o.MachineSets) == 0 {
		return fmt.Errorf("--machinesets is required")
	}
	return nil
}

// Run connects to spoke, discovers MachineSets, extracts config, and creates the Hive MachinePool.
func (o *AdoptOptions) Run() error {
	cd, err := GetClusterDeployment(o.Namespace, o.ClusterDeploymentName)
	if err != nil {
		return err
	}
	if cd.DeletionTimestamp != nil {
		return fmt.Errorf("ClusterDeployment %s/%s is being deleted", o.Namespace, o.ClusterDeploymentName)
	}
	if !cd.Spec.Installed {
		return fmt.Errorf("ClusterDeployment %s/%s is not installed yet", o.Namespace, o.ClusterDeploymentName)
	}
	if cd.Annotations != nil && cd.Annotations[constants.RelocateAnnotation] != "" {
		return fmt.Errorf("ClusterDeployment %s/%s is relocating", o.Namespace, o.ClusterDeploymentName)
	}
	if cd.Spec.ClusterMetadata == nil || cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name == "" {
		return fmt.Errorf("ClusterDeployment %s/%s has no ClusterMetadata.AdminKubeconfigSecretRef (spoke not installed or not adopted)", o.Namespace, o.ClusterDeploymentName)
	}

	hubClient, err := utils.GetClient(adoptControllerName)
	if err != nil {
		return errors.Wrap(err, "get hub client")
	}

	remoteBuilder := remoteclient.NewBuilder(hubClient, cd, adoptControllerName)
	remoteClient, err := remoteBuilder.Build()
	if err != nil {
		return errors.Wrap(err, "build spoke client")
	}

	machineSets, err := o.getMachineSets(remoteClient)
	if err != nil {
		return err
	}
	if len(machineSets) == 0 {
		return fmt.Errorf("no MachineSets found with names %v in namespace %s", o.MachineSets, machineAPINamespace)
	}
	o.log.Infof("Found %d MachineSet(s) to adopt", len(machineSets))

	result, err := ExtractFromMachineSets(machineSets)
	if err != nil {
		return errors.Wrap(err, "extract config from MachineSets")
	}

	if err := validateFailureDomainsForAdopt(cd, result, machineSets, remoteClient); err != nil {
		return errors.Wrap(err, "validate CD and MachineSet together (platform + failure domains)")
	}

	opts := &machinepoolresource.BuildOptions{Replicas: ptr.To(result.TotalReplicas)}
	mp := machinepoolresource.BuildMachinePool(cd.Name, cd.Namespace, o.PoolName, opts, result.Filler)
	if mp == nil {
		return fmt.Errorf("failed to build MachinePool")
	}

	if o.Output != "" {
		var printer printers.ResourcePrinter
		if o.Output == "json" {
			printer = &printers.JSONPrinter{}
		} else {
			printer = &printers.YAMLPrinter{}
		}
		return printer.PrintObj(mp, nil)
	}

	// Auto pause CD before applying
	o.log.Info("Pausing ClusterDeployment reconciliation...")
	if err := PauseClusterDeployment(hubClient, cd); err != nil {
		return errors.Wrap(err, "pause ClusterDeployment")
	}
	// Auto resume after apply
	defer func() {
		o.log.Info("Resuming ClusterDeployment reconciliation...")
		cdRefreshed, _ := GetClusterDeployment(o.Namespace, o.ClusterDeploymentName)
		if cdRefreshed != nil {
			if err := ResumeClusterDeployment(hubClient, cdRefreshed); err != nil {
				o.log.WithError(err).Warn("failed to resume ClusterDeployment")
			}
		}
	}()

	// Label MachineSets on spoke cluster so Hive recognizes them
	o.log.Info("Labeling MachineSets on spoke cluster...")
	n, err := LabelMachineSetsForPool(context.Background(), remoteClient, machineSets, o.PoolName, o.log)
	if err != nil {
		return errors.Wrap(err, "label MachineSets on spoke")
	}
	o.log.Infof("Labeled %d MachineSet(s) with %s=%s", n, machinePoolNameLabel, o.PoolName)

	rh, err := utils.GetResourceHelper(hivev1.ControllerName(adoptControllerName), o.log)
	if err != nil {
		return errors.Wrap(err, "get resource helper")
	}
	if _, err := rh.ApplyRuntimeObject(mp, scheme.GetScheme()); err != nil {
		return errors.Wrap(err, "apply MachinePool")
	}
	o.log.Infof("MachinePool %s/%s adopted (platform=%s, replicas=%d)", mp.Namespace, mp.Name, result.Platform, result.TotalReplicas)
	return nil
}

// getMachineSets returns MachineSets from the spoke by name.
func (o *AdoptOptions) getMachineSets(remoteClient client.Client) ([]*machineapi.MachineSet, error) {
	ctx := context.Background()
	out := make([]*machineapi.MachineSet, 0, len(o.MachineSets))
	for _, name := range o.MachineSets {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ms := &machineapi.MachineSet{}
		if err := remoteClient.Get(ctx, client.ObjectKey{Namespace: machineAPINamespace, Name: name}, ms); err != nil {
			return nil, errors.Wrapf(err, "get MachineSet %s/%s", machineAPINamespace, name)
		}
		out = append(out, ms)
	}
	return out, nil
}

// validateFailureDomainsForAdopt validates CD and MachineSet(s) together: (1) CD platform must match
// MachineSet(s) platform; (2) for Nutanix, extracted FD names must exist in CD; for vSphere, each
// MS Workspace must match some spoke Infrastructure FD. Skips FD checks for other platforms.
func validateFailureDomainsForAdopt(cd *hivev1.ClusterDeployment, result ExtractResult, machineSets []*machineapi.MachineSet, remoteClient client.Client) error {
	cdPlatform, err := PlatformFromCD(cd)
	if err != nil {
		return err
	}
	if cdPlatform != result.Platform {
		return fmt.Errorf("ClusterDeployment platform %q does not match MachineSet(s) platform %q", cdPlatform, result.Platform)
	}
	switch result.Platform {
	case constants.PlatformNutanix:
		return validateNutanixFailureDomains(cd, result)
	case constants.PlatformVSphere:
		return validateVSphereFailureDomains(machineSets, remoteClient)
	default:
		return nil
	}
}

func validateNutanixFailureDomains(cd *hivev1.ClusterDeployment, result ExtractResult) error {
	if cd.Spec.Platform.Nutanix == nil {
		return fmt.Errorf("ClusterDeployment has no Nutanix platform; cannot validate failure domains")
	}
	opts, ok := result.Filler.(*machinepoolresource.NutanixOptions)
	if !ok {
		return nil
	}
	if len(opts.FailureDomains) == 0 {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, fd := range cd.Spec.Platform.Nutanix.FailureDomains {
		allowed[fd.Name] = struct{}{}
	}
	for _, fd := range opts.FailureDomains {
		if _, ok := allowed[fd]; !ok {
			return fmt.Errorf("failure domain %q from MachineSet(s) is not in ClusterDeployment.Spec.Platform.Nutanix.FailureDomains", fd)
		}
	}
	return nil
}

func validateVSphereFailureDomains(machineSets []*machineapi.MachineSet, remoteClient client.Client) error {
	infra, err := getInfrastructure(remoteClient)
	if err != nil {
		return errors.Wrap(err, "fetch Infrastructure for vSphere failure domain validation")
	}
	if infra.Spec.PlatformSpec.VSphere == nil || len(infra.Spec.PlatformSpec.VSphere.FailureDomains) == 0 {
		return nil
	}
	fds := infra.Spec.PlatformSpec.VSphere.FailureDomains
	for _, ms := range machineSets {
		cfg := &machineapi.VSphereMachineProviderSpec{}
		if ms.Spec.Template.Spec.ProviderSpec.Value == nil || len(ms.Spec.Template.Spec.ProviderSpec.Value.Raw) == 0 {
			return fmt.Errorf("MachineSet %s/%s has no providerSpec", ms.Namespace, ms.Name)
		}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
			return errors.Wrapf(err, "decode MachineSet %s providerSpec", ms.Name)
		}
		ws := cfg.Workspace
		if ws == nil {
			return fmt.Errorf("MachineSet %s has no Workspace; cannot validate failure domain", ms.Name)
		}
		var matched bool
		for i := range fds {
			fd := &fds[i]
			vmGroup := ""
			if fd.ZoneAffinity != nil && fd.ZoneAffinity.HostGroup != nil && fd.ZoneAffinity.HostGroup.VMGroup != "" {
				vmGroup = fd.ZoneAffinity.HostGroup.VMGroup
			}
			fdResourcePool := fd.Topology.ResourcePool
			if fdResourcePool == "" && fd.Topology.ComputeCluster != "" {
				fdResourcePool = path.Clean(fmt.Sprintf("/%s/Resources", fd.Topology.ComputeCluster))
			}
			if ws.Datacenter == fd.Topology.Datacenter &&
				ws.Datastore == fd.Topology.Datastore &&
				ws.Server == fd.Server &&
				ws.VMGroup == vmGroup &&
				path.Clean(ws.ResourcePool) == path.Clean(fdResourcePool) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("MachineSet %s Workspace (datacenter=%q, datastore=%q, server=%q, resourcePool=%q) does not match any failure domain in spoke Infrastructure", ms.Name, ws.Datacenter, ws.Datastore, ws.Server, ws.ResourcePool)
		}
	}
	return nil
}

func getInfrastructure(c client.Client) (*configv1.Infrastructure, error) {
	infra := &configv1.Infrastructure{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(configv1.SchemeGroupVersion.WithKind("Infrastructure"))
	if err := c.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infra, &client.GetOptions{Raw: &metav1.GetOptions{TypeMeta: tm}}); err != nil {
		return nil, err
	}
	return infra, nil
}
