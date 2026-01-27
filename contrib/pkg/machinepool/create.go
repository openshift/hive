package machinepool

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/machinepool/internal"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/util/scheme"
)

var validTaintEffects = map[corev1.TaintEffect]bool{
	corev1.TaintEffectNoSchedule:       true,
	corev1.TaintEffectPreferNoSchedule: true,
	corev1.TaintEffectNoExecute:        true,
}

type CreateOptions struct {
	*internal.CreateOptionsWithEmbeddedAPI

	// Output options
	DryRun bool
	Output string

	// Internal fields
	log          log.FieldLogger
	hubClient    client.Client
	parsedTaints []corev1.Taint
}

// NewCreateCommand creates the cobra command for creating MachinePools.
func NewCreateCommand() *cobra.Command {
	opt := &CreateOptions{
		CreateOptionsWithEmbeddedAPI: &internal.CreateOptionsWithEmbeddedAPI{
			Labels:        make(map[string]string),
			MachineLabels: make(map[string]string),
		},
		log: log.WithField("command", "machinepool create"),
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new Hive MachinePool",
		Long: `Create a new Hive MachinePool with user-provided configuration.

Supports AWS, GCP, Azure, OpenStack, vSphere, Nutanix, and IBM Cloud.
Use --output (yaml|json) to preview, or omit --output to apply to cluster.`,
		Example: `
  hiveutil machinepool create -c mycluster -n mynamespace --pool worker --platform aws --aws-type m5.large --zones us-east-1a,us-east-1b --replicas 3 --aws-subnets subnet-1,subnet-2 --aws-ec2-root-volume-size 120 --aws-ec2-root-volume-type gp3 --output yaml
  hiveutil machinepool create -c mycluster -n mynamespace --pool worker --platform gcp --gcp-type n1-standard-4 --zones us-central1-a,us-central1-b --replicas 3 --gcp-os-disk-disk-size-gb 256 --gcp-os-disk-disk-type pd-ssd
  hiveutil machinepool create -c mycluster -n mynamespace --pool worker --platform azure --azure-type Standard_D2s_v3 --zones 1,2,3 --replicas 3 --azure-os-disk-disk-size-gb 128 --azure-os-disk-disk-type Premium_LRS --azure-compute-subnet subnet-1`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := internal.PostParseFlags(); err != nil {
				opt.log.WithError(err).Fatal("Failed to post-process flags")
			}
			if err := opt.Complete(); err != nil {
				opt.log.WithError(err).Fatal("Failed to complete options")
			}
			if err := opt.Validate(); err != nil {
				opt.log.WithError(err).Fatal("Invalid options")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Failed to create machinepool")
			}
		},
	}

	if err := internal.AddFlags(opt.CreateOptionsWithEmbeddedAPI, cmd); err != nil {
		panic(fmt.Sprintf("Failed to add flags: %v", err))
	}

	flags := cmd.Flags()
	flags.BoolVar(&opt.DryRun, "dry-run", false, "Show what will be done without making changes (same as --output yaml)")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output format (yaml|json). If not specified, resources will be applied to cluster")

	cmd.MarkFlagRequired("cluster-deployment")
	cmd.MarkFlagRequired("pool")
	cmd.MarkFlagRequired("platform")
	return cmd
}

// Complete initializes the CreateOptions with required clients and parsed data.
func (o *CreateOptions) Complete() error {
	client, err := utils.GetClient("machinepool-create")
	if err != nil {
		return err
	}
	o.hubClient = client

	if len(o.Taints) > 0 {
		parsed, err := parseTaints(o.Taints)
		if err != nil {
			return errors.Wrap(err, "failed to parse taints")
		}
		o.parsedTaints = parsed
	}
	return nil
}

// Validate validates all CreateOptions fields.
func (o *CreateOptions) Validate() error {
	if o.DryRun && o.Output == "" {
		o.Output = "yaml"
	}

	var errs []string
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		errs = append(errs, fmt.Sprintf("invalid output format: %s. Valid formats: yaml, json", o.Output))
	}

	normalizedPlatform := strings.ToLower(o.Platform)
	if _, err := GetPlatformHandler(normalizedPlatform); err != nil {
		errs = append(errs, fmt.Sprintf("invalid platform: %s. Valid platforms: aws, gcp, azure, openstack, vsphere, nutanix, ibmcloud", o.Platform))
	} else {
		o.Platform = normalizedPlatform
	}

	if o.Replicas < 0 {
		errs = append(errs, "--replicas must be non-negative")
	}

	if err := o.validatePlatformSpecificFields(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := o.validateAndSanitizeInputs(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// validatePlatformSpecificFields validates platform-specific configuration.
func (o *CreateOptions) validatePlatformSpecificFields() error {
	handler, err := GetPlatformHandler(o.Platform)
	if err != nil {
		return nil
	}
	return handler.ValidatePlatformSpecificFields(o)
}

// validateAndSanitizeInputs validates and sanitizes user inputs (tags, labels, etc.).
func (o *CreateOptions) validateAndSanitizeInputs() error {
	if o.AWS != nil && o.AWS.UserTags != nil {
		for key := range o.AWS.UserTags {
			if len(key) == 0 {
				return errors.New("AWS tag key cannot be empty")
			}
			keyLower := strings.ToLower(key)
			if strings.HasPrefix(keyLower, "aws:") || strings.HasPrefix(keyLower, "kubernetes.io/") {
				return errors.Errorf("AWS tag key %q cannot start with reserved prefix", key)
			}
		}
	}

	for key, value := range o.Labels {
		if err := validateKubernetesLabel(key, value); err != nil {
			return errors.Wrapf(err, "invalid label %s=%s", key, value)
		}
	}
	for key, value := range o.MachineLabels {
		if err := validateKubernetesLabel(key, value); err != nil {
			return errors.Wrapf(err, "invalid machine label %s=%s", key, value)
		}
	}

	return nil
}

// validateAgainstClusterDeployment validates options against the ClusterDeployment.
func (o *CreateOptions) validateAgainstClusterDeployment(cd *hivev1.ClusterDeployment) error {
	cdPlatform := GetPlatformFromClusterDeployment(cd)
	if cdPlatform == constants.PlatformUnknown {
		return errors.Errorf("ClusterDeployment %s/%s has no platform configuration", cd.Namespace, o.ClusterDeploymentName)
	}
	if cdPlatform != o.Platform {
		return errors.Errorf("platform mismatch: ClusterDeployment uses %s, but --platform specifies %s", cdPlatform, o.Platform)
	}

	handler, err := GetPlatformHandler(o.Platform)
	if err != nil {
		return errors.Wrapf(err, "unsupported platform: %s", o.Platform)
	}
	if !handler.SupportsZones() && len(o.Zones) > 0 {
		o.log.WithField("platform", o.Platform).Warn("Platform does not support zones. --zones flag will be ignored.")
		return nil
	}
	if o.Platform != constants.PlatformNutanix || o.Nutanix == nil || len(o.Nutanix.FailureDomains) == 0 {
		return nil
	}
	if cd.Spec.Platform.Nutanix == nil || len(cd.Spec.Platform.Nutanix.FailureDomains) == 0 {
		return errors.Errorf("Nutanix failure domains specified but ClusterDeployment %s/%s has none defined",
			cd.Namespace, o.ClusterDeploymentName)
	}
	for _, userFD := range o.Nutanix.FailureDomains {
		if err := handler.ValidateFailureDomain(userFD, cd); err != nil {
			return err
		}
	}
	return nil
}

// Run executes the create command.
func (o *CreateOptions) Run() error {
	objs, err := o.GenerateObjects()
	if err != nil {
		return err
	}

	s := scheme.GetScheme()
	if o.Output != "" {
		PrintObjects(objs, s, CreatePrinter(o.Output))
		return nil
	}

	rh, err := utils.GetResourceHelper("machinepool-create", o.log)
	if err != nil {
		return err
	}

	for _, obj := range objs {
		if _, err := rh.ApplyRuntimeObject(obj, s); err != nil {
			return err
		}
	}
	return nil
}

// GenerateObjects generates the MachinePool objects to be created.
func (o *CreateOptions) GenerateObjects() ([]runtime.Object, error) {
	if o.Namespace == "" {
		ns, err := utils.DefaultNamespace()
		if err != nil {
			return nil, errors.Wrap(err, "cannot determine default namespace")
		}
		o.Namespace = ns
	}

	cd := &hivev1.ClusterDeployment{}
	key := types.NamespacedName{Namespace: o.Namespace, Name: o.ClusterDeploymentName}
	if err := o.hubClient.Get(context.Background(), key, cd); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.Errorf("ClusterDeployment %s/%s not found", o.Namespace, o.ClusterDeploymentName)
		}
		return nil, errors.Wrapf(err, "failed to get ClusterDeployment %s/%s", o.Namespace, o.ClusterDeploymentName)
	}

	if !cd.Spec.Installed {
		return nil, errors.Errorf("ClusterDeployment %s/%s is not installed yet. MachinePools can only be created for installed clusters", o.Namespace, o.ClusterDeploymentName)
	}
	if cd.DeletionTimestamp != nil {
		return nil, errors.Errorf("ClusterDeployment %s/%s is being deleted", o.Namespace, o.ClusterDeploymentName)
	}

	if err := o.validateAgainstClusterDeployment(cd); err != nil {
		return nil, err
	}

	replicas := o.Replicas
	machinePool, err := BuildMachinePool(&MachinePoolConfig{
		ClusterDeployment: cd,
		PoolName:          o.PoolName,
		Replicas:          &replicas,
		Platform:          o.Platform,
		CreateOptions:     o,
		Labels:            o.Labels,
		MachineLabels:     o.MachineLabels,
		Taints:            o.parsedTaints,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to build MachinePool")
	}
	return []runtime.Object{machinePool}, nil
}

// Helper functions

// parseTaints parses taint strings into corev1.Taint objects.
// Format: "key=value:effect" where effect is NoSchedule, PreferNoSchedule, or NoExecute.
func parseTaints(taintStrings []string) ([]corev1.Taint, error) {
	taints := make([]corev1.Taint, 0, len(taintStrings))
	for _, taintStr := range taintStrings {
		parts := strings.SplitN(taintStr, ":", 2)
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid taint format: %s (expected key=value:effect)", taintStr)
		}
		keyValue := strings.SplitN(parts[0], "=", 2)
		if len(keyValue) != 2 {
			return nil, errors.Errorf("invalid taint format: %s (expected key=value:effect)", taintStr)
		}
		effect := corev1.TaintEffect(parts[1])
		if !validTaintEffects[effect] {
			return nil, errors.Errorf("invalid taint effect: %s", parts[1])
		}
		taints = append(taints, corev1.Taint{Key: keyValue[0], Value: keyValue[1], Effect: effect})
	}
	return taints, nil
}

// validateKubernetesLabel validates a Kubernetes label key and value.
func validateKubernetesLabel(key, value string) error {
	const maxLabelKeyLength = 253
	const maxLabelValueLength = 63
	if key == "" {
		return errors.New("label key cannot be empty")
	}
	var name string
	if strings.Contains(key, "/") {
		parts := strings.SplitN(key, "/", 2)
		if len(parts[0]) > maxLabelKeyLength {
			return errors.Errorf("label prefix %q exceeds max length %d", parts[0], maxLabelKeyLength)
		}
		name = parts[1]
	} else {
		name = key
	}
	if name == "" {
		return errors.New("label key name cannot be empty")
	}
	if len(name) > maxLabelValueLength {
		return errors.Errorf("label key name %q exceeds max length %d", name, maxLabelValueLength)
	}
	if len(value) > maxLabelValueLength {
		return errors.Errorf("label value for %q exceeds max length %d", key, maxLabelValueLength)
	}
	return nil
}
