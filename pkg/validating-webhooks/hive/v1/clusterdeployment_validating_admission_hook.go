package v1

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/aws"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/apis/hive/v1/gcp"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/awsprivatelink"
	"github.com/openshift/hive/pkg/manageddns"
	"github.com/openshift/hive/pkg/util/contracts"
)

const (
	clusterDeploymentGroup    = "hive.openshift.io"
	clusterDeploymentVersion  = "v1"
	clusterDeploymentResource = "clusterdeployments"

	clusterDeploymentAdmissionGroup   = "admission.hive.openshift.io"
	clusterDeploymentAdmissionVersion = "v1"
)

var (
	mutableFields = []string{
		"CertificateBundles",
		"ClusterMetadata",
		"ControlPlaneConfig",
		"Ingress",
		"Installed",
		"PreserveOnDelete",
		"ClusterPoolRef",
		"PowerState",
		"HibernateAfter",
		"InstallAttemptsLimit",
		"Platform.AgentBareMetal.AgentSelector",
		"Platform.AWS.PrivateLink.AdditionalAllowedPrincipals",
		"Platform.GCP.DiscardLocalSsdOnHibernate",
	}
)

// ClusterDeploymentValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterDeploymentValidatingAdmissionHook struct {
	decoder admission.Decoder

	validManagedDomains  []string
	fs                   *featureSet
	awsPrivateLinkConfig *hivev1.AWSPrivateLinkConfig
	supportedContracts   contracts.SupportedContractImplementationsList
}

// NewClusterDeploymentValidatingAdmissionHook constructs a new ClusterDeploymentValidatingAdmissionHook
func NewClusterDeploymentValidatingAdmissionHook(decoder admission.Decoder) *ClusterDeploymentValidatingAdmissionHook {
	logger := log.WithField("validatingWebhook", "clusterdeployment")
	managedDomains, err := manageddns.ReadManagedDomainsFile()
	if err != nil {
		logger.WithError(err).Fatal("Unable to read managedDomains file")
	}
	domains := []string{}
	for _, md := range managedDomains {
		domains = append(domains, md.Domains...)
	}

	aplConfig, err := awsprivatelink.ReadAWSPrivateLinkControllerConfigFile()
	if err != nil {
		logger.WithError(err).Fatal("Unable to read AWS Private Link Config file")
	}

	supportContractsConfig, err := contracts.ReadSupportContractsFile()
	if err != nil {
		logger.WithError(err).Fatal("Unable to read Supported Contract Implementations file")

	}

	logger.WithField("managedDomains", domains).Info("Read managed domains")
	return &ClusterDeploymentValidatingAdmissionHook{
		decoder:              decoder,
		validManagedDomains:  domains,
		fs:                   newFeatureSet(),
		awsPrivateLinkConfig: aplConfig,
		supportedContracts:   supportContractsConfig,
	}
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/clusterdeploymentvalidators".
// When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *ClusterDeploymentValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": "clusterdeploymentvalidator",
	}).Info("Registering validation REST resource")

	// NOTE: This GVR is meant to be different than the ClusterDeployment CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    clusterDeploymentAdmissionGroup,
			Version:  clusterDeploymentAdmissionVersion,
			Resource: "clusterdeploymentvalidators",
		},
		"clusterdeploymentvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterDeploymentValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": "clusterdeploymentvalidator",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *ClusterDeploymentValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "Validate",
	})

	if !a.shouldValidate(admissionSpec) {
		contextLogger.Info("Skipping validation for request")
		// The request object isn't something that this validator should validate.
		// Therefore, we say that it's Allowed.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	contextLogger.Info("Validating request")

	switch admissionSpec.Operation {
	case admissionv1beta1.Create:
		return a.validateCreate(admissionSpec)
	case admissionv1beta1.Update:
		return a.validateUpdate(admissionSpec)
	case admissionv1beta1.Delete:
		return a.validateDelete(admissionSpec)
	default:
		contextLogger.Info("Successful validation")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}

// shouldValidate explicitly checks if the request should validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *ClusterDeploymentValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != clusterDeploymentGroup {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != clusterDeploymentVersion {
		contextLogger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != clusterDeploymentResource {
		contextLogger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for ClusterDeployment objects.
func (a *ClusterDeploymentValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	if admResp := validatefeatureGates(a.decoder, admissionSpec, a.fs, contextLogger); admResp != nil {
		contextLogger.Errorf("object was rejected due to feature gate failures")
		return admResp
	}

	cd := &hivev1.ClusterDeployment{}
	if err := a.decoder.DecodeRaw(admissionSpec.Object, cd); err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	dr := creationHooksDisabled(cd)

	// Add the new data to the contextLogger
	contextLogger.Data["object.Name"] = cd.Name

	// TODO: Put Create Validation Here (or in openAPIV3Schema validation section of crd)

	if len(cd.Name) > validation.DNS1123LabelMaxLength {
		message := fmt.Sprintf("Invalid cluster deployment name (.meta.name): %s", validation.MaxLenError(validation.DNS1123LabelMaxLength))
		contextLogger.Error(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if len(cd.Spec.ClusterName) > validation.DNS1123LabelMaxLength {
		message := fmt.Sprintf("Invalid cluster name (.spec.clusterName): %s", validation.MaxLenError(validation.DNS1123LabelMaxLength))
		contextLogger.Error(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// validate the ingress
	if ingressValidationResult := validateIngress(cd, contextLogger); ingressValidationResult != nil {
		return ingressValidationResult
	}

	// validate the certificate bundles
	if r := validateCertificateBundles(cd, contextLogger); r != nil {
		return r
	}

	if cd.Spec.ManageDNS {
		if !validateDomain(cd.Spec.BaseDomain, a.validManagedDomains) {
			message := "The base domain must be a child of one of the managed domains for ClusterDeployments with manageDNS set to true"
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
	}

	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	if !cd.Spec.Installed {
		if cd.Spec.Provisioning != nil && cd.Spec.ClusterInstallRef != nil {
			allErrs = append(allErrs, field.Forbidden(specPath.Child("provisioning"), "provisioning and clusterInstallRef cannot be set at the same time"))
		}

		if cd.Spec.Provisioning == nil && cd.Spec.ClusterInstallRef == nil {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning"), "provisioning is required if not installed"))
		}
	}

	if !cd.Spec.Installed && cd.Spec.Provisioning != nil {
		// InstallConfigSecretRef is not required for anyone using the new ClusterInstall interface:
		if cd.Spec.Provisioning.InstallConfigSecretRef == nil || cd.Spec.Provisioning.InstallConfigSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning", "installConfigSecretRef", "name"), "must specify an InstallConfig"))
		}
	}

	allErrs = append(allErrs, validateClusterPlatform(specPath.Child("platform"), cd.Spec.Platform)...)
	allErrs = append(allErrs, validateCanManageDNSForClusterPlatform(specPath, cd.Spec)...)

	if cd.Spec.Platform.AWS != nil {
		allErrs = append(allErrs, validateAWSPrivateLink(specPath.Child("platform", "aws"), cd.Spec.Platform.AWS, a.awsPrivateLinkConfig)...)
	}

	if cd.Spec.Provisioning != nil {
		if cd.Spec.Provisioning.SSHPrivateKeySecretRef != nil && cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning", "sshPrivateKeySecretRef", "name"), "must specify a name for the ssh private key secret if the ssh private key secret is specified"))
		}
		if cd.Spec.Platform.IBMCloud != nil && cd.Spec.Provisioning.ManifestsConfigMapRef == nil && cd.Spec.Provisioning.ManifestsSecretRef == nil {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning"), "must specify manifestsConfigMapRef or manifestsSecretRef when platform is IBM Cloud"))
		}
		if cd.Spec.Provisioning.ManifestsConfigMapRef != nil && cd.Spec.Provisioning.ManifestsSecretRef != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("provisioning", "manifestsConfigMapRef"), cd.Spec.Provisioning.ManifestsConfigMapRef.Name, "manifestsConfigMapRef and manifestsSecretRef are mutually exclusive"))
		}
	}

	if cd.Spec.ClusterInstallRef != nil {
		supported := a.supportedContracts.SupportedImplementations(hivecontractsv1alpha1.ClusterInstallContractName)
		if len(supported) == 0 {
			allErrs = append(allErrs, field.Forbidden(specPath.Child("clusterInstallRef"), "there are no supported implementations of clusterinstall contract"))
		} else {
			ref := cd.Spec.ClusterInstallRef
			if !a.supportedContracts.IsSupported(hivecontractsv1alpha1.ClusterInstallContractName,
				contracts.ContractImplementation{Group: ref.Group, Version: ref.Version, Kind: ref.Kind}) {
				allErrs = append(allErrs, field.NotSupported(specPath.Child("clusterInstallRef", "kind"), ref.Kind, supported))
			}
		}
	}

	// Disable this check for DR
	if !dr {
		if poolRef := cd.Spec.ClusterPoolRef; poolRef != nil {
			if claimName := poolRef.ClaimName; claimName != "" {
				allErrs = append(allErrs, field.Invalid(specPath.Child("clusterPoolRef", "claimName"), claimName, "cannot create a ClusterDeployment that is already claimed"))
			}
		}
	}

	if len(allErrs) > 0 {
		status := errors.NewInvalid(schemaGVK(admissionSpec.Kind).GroupKind(), admissionSpec.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

func validateAWSPrivateLink(path *field.Path, platform *hivev1aws.Platform, config *hivev1.AWSPrivateLinkConfig) field.ErrorList {
	allErrs := field.ErrorList{}
	pl := platform.PrivateLink

	if pl == nil || !pl.Enabled {
		return allErrs
	}

	if config == nil || len(config.EndpointVPCInventory) == 0 {
		allErrs = append(allErrs, field.Forbidden(path.Child("privateLink", "enabled"), "AWS PrivateLink is not supported in the environment"))
		return allErrs
	}

	supportedRegions := sets.NewString()
	for _, inv := range config.EndpointVPCInventory {
		supportedRegions.Insert(inv.Region)
	}
	if !supportedRegions.Has(platform.Region) {
		allErrs = append(allErrs, field.Forbidden(path.Child("privateLink", "enabled"),
			fmt.Sprintf("AWS Private Link is not supported in %s region", platform.Region)))
	}

	return allErrs
}

/* TODO: move to explicit validation for AgentClusterInstall */
/*
func validateAgentInstallStrategy(specPath *field.Path, cd *hivev1.ClusterDeployment) field.ErrorList {
	ais := cd.Spec.Provisioning.InstallStrategy.Agent
	allErrs := field.ErrorList{}
	agentPath := specPath.Child("provisioning", "installStrategy", "agent")

	// agent install strategy can only be used with agent bare metal platform today:
	if cd.Spec.Platform.AgentBareMetal == nil {
		allErrs = append(allErrs,
			field.Forbidden(agentPath,
				"agent install strategy can only be used with agent bare metal platform"))
	}

	// must use either 1 or 3 control plane agents:
	if ais.ProvisionRequirements.ControlPlaneAgents != 1 &&
		ais.ProvisionRequirements.ControlPlaneAgents != 3 {
		allErrs = append(allErrs, field.Invalid(
			agentPath.Child("provisionRequirements", "controlPlaneAgents"),
			ais.ProvisionRequirements.ControlPlaneAgents,
			"cluster can only be formed with 1 or 3 control plane agents"))
	}

	// must use either 0 or >=2 worker agents due to limitations in assisted service:
	if ais.ProvisionRequirements.WorkerAgents == 1 {
		allErrs = append(allErrs, field.Invalid(
			agentPath.Child("provisionRequirements", "workerAgents"),
			ais.ProvisionRequirements.WorkerAgents,
			"cluster can only be formed with 0 or >= 2 worker agents"))
	}

	// install config secret ref should not be set for agent installs:
	if cd.Spec.Provisioning.InstallConfigSecretRef != nil {
		allErrs = append(allErrs,
			field.Forbidden(specPath.Child("provisioning", "installConfigSecretRef"),
				"custom install config cannot be used with agent install strategy"))
	}
	return allErrs
}
*/

func validatefeatureGates(decoder admission.Decoder, admissionSpec *admissionv1beta1.AdmissionRequest, fs *featureSet, contextLogger *log.Entry) *admissionv1beta1.AdmissionResponse {
	obj := &unstructured.Unstructured{}
	if err := decoder.DecodeRaw(admissionSpec.Object, obj); err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	contextLogger.WithField("enabledFeatureGates", fs.Enabled).Info("feature gates enabled")

	errs := field.ErrorList{}
	// To add validation for feature gates use these examples
	// 		errs = append(errs, equalOnlyWhenFeatureGate(fs, obj, "spec.platform.type", "AlphaPlatformAEnabled", "platformA")...)

	if len(errs) > 0 && len(errs.ToAggregate().Errors()) > 0 {
		status := errors.NewInvalid(schemaGVK(admissionSpec.Kind).GroupKind(), admissionSpec.Name, errs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	return nil
}

func validateClusterPlatform(path *field.Path, platform hivev1.Platform) field.ErrorList {
	allErrs := field.ErrorList{}
	numberOfPlatforms := 0
	if aws := platform.AWS; aws != nil {
		numberOfPlatforms++
		awsPath := path.Child("aws")
		if aws.CredentialsSecretRef.Name == "" && aws.CredentialsAssumeRole == nil {
			allErrs = append(allErrs, field.Required(awsPath.Child("credentialsSecretRef", "name"), "must specify secrets for AWS access"))
		}
		if aws.CredentialsAssumeRole != nil && aws.CredentialsSecretRef.Name != "" {
			allErrs = append(allErrs, field.Required(awsPath.Child("credentialsAssumeRole"), "cannot specify assume role when credentials secret is provided"))
		}
		if aws.Region == "" {
			allErrs = append(allErrs, field.Required(awsPath.Child("region"), "must specify AWS region"))
		}
	}
	if azure := platform.Azure; azure != nil {
		numberOfPlatforms++
		azurePath := path.Child("azure")
		if azure.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("credentialsSecretRef", "name"), "must specify secrets for Azure access"))
		}
		if azure.Region == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("region"), "must specify Azure region"))
		}
		if azure.BaseDomainResourceGroupName == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("baseDomainResourceGroupName"), "must specify the Azure resource group for the base domain"))
		}
	}
	if gcp := platform.GCP; gcp != nil {
		numberOfPlatforms++
		gcpPath := path.Child("gcp")
		if gcp.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("credentialsSecretRef", "name"), "must specify secrets for GCP access"))
		}
		if gcp.Region == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("region"), "must specify GCP region"))
		}
	}
	if openstack := platform.OpenStack; openstack != nil {
		numberOfPlatforms++
		openstackPath := path.Child("openStack")
		if openstack.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(openstackPath.Child("credentialsSecretRef", "name"), "must specify secrets for OpenStack access"))
		}
		if openstack.CertificatesSecretRef != nil && openstack.CertificatesSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(openstackPath.Child("certificatesSecretRef", "name"), "must specify name of the secret for OpenStack access"))
		}
		if openstack.Cloud == "" {
			allErrs = append(allErrs, field.Required(openstackPath.Child("cloud"), "must specify cloud section of credentials secret to use"))
		}
	}
	if vsphere := platform.VSphere; vsphere != nil {
		numberOfPlatforms++
		vspherePath := path.Child("vsphere")
		if vsphere.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(vspherePath.Child("credentialsSecretRef", "name"), "must specify secrets for vSphere access"))
		}
		if vsphere.CertificatesSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(vspherePath.Child("certificatesSecretRef", "name"), "must specify certificates for vSphere access"))
		}
		if vsphere.VCenter == "" {
			allErrs = append(allErrs, field.Required(vspherePath.Child("vCenter"), "must specify vSphere vCenter"))
		}
		if vsphere.Datacenter == "" {
			allErrs = append(allErrs, field.Required(vspherePath.Child("datacenter"), "must specify vSphere datacenter"))
		}
		if vsphere.DefaultDatastore == "" {
			allErrs = append(allErrs, field.Required(vspherePath.Child("defaultDatastore"), "must specify vSphere defaultDatastore"))
		}
	}
	if nutanix := platform.Nutanix; nutanix != nil {
		numberOfPlatforms++
		nutanixPath := path.Child("nutanix")
		if nutanix.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(nutanixPath.Child("credentialsSecretRef", "name"), "must specify secrets for Nutanix access"))
		}
		if nutanix.PrismCentral.Endpoint.Address == "" {
			allErrs = append(allErrs, field.Required(nutanixPath.Child("endpointAddress"), "must specify Nutanix Prism Central endpoint address"))
		}
		if nutanix.PrismCentral.Endpoint.Port == 0 {
			allErrs = append(allErrs, field.Required(nutanixPath.Child("endpointAddress"), "must specify Nutanix Prism Central endpoint port"))
		}
		// TODO
		//if nutanix.Cluster == "" {
		//	allErrs = append(allErrs, field.Required(nutanixPath.Child("cluster"), "must specify Nutanix Prism Central cluster"))
		//}
		if len(nutanix.SubnetUUIDs) == 0 {
			allErrs = append(allErrs, field.Required(nutanixPath.Child("subnetUUIDs"), "must specify at least one Nutanix Prism Central Subnet UUID"))
		}
	}
	if ovirt := platform.Ovirt; ovirt != nil {
		numberOfPlatforms++
		ovirtPath := path.Child("ovirt")
		if ovirt.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(ovirtPath.Child("credentialsSecretRef", "name"), "must specify secrets for oVirt access"))
		}
		if ovirt.CertificatesSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(ovirtPath.Child("certificatesSecretRef", "name"), "must specify certificates for oVirt access"))
		}
		if ovirt.ClusterID == "" {
			allErrs = append(allErrs, field.Required(ovirtPath.Child("ovirt_cluster_id"), "must specify ovirt_cluster_id"))
		}
		if ovirt.StorageDomainID == "" {
			allErrs = append(allErrs, field.Required(ovirtPath.Child("ovirt_storage_domain_id"), "must specify ovirt_storage_domain_id"))
		}
	}
	if ibmCloud := platform.IBMCloud; ibmCloud != nil {
		numberOfPlatforms++
		ibmCloudPath := path.Child("ibmcloud")
		if ibmCloud.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(ibmCloudPath.Child("credentialsSecretRef", "name"), "must specify secrets for IBM access"))
		}
		if ibmCloud.Region == "" {
			allErrs = append(allErrs, field.Required(ibmCloudPath.Child("region"), "must specify IBM region"))
		}
	}
	if platform.BareMetal != nil {
		numberOfPlatforms++
	}
	if platform.AgentBareMetal != nil {
		numberOfPlatforms++
	}
	if platform.None != nil {
		numberOfPlatforms++
	}
	switch {
	case numberOfPlatforms == 0:
		allErrs = append(allErrs, field.Required(path, "must specify a platform"))
	case numberOfPlatforms > 1:
		allErrs = append(allErrs, field.Invalid(path, platform, "must specify only a single platform"))
	}
	return allErrs
}

func validateCanManageDNSForClusterPlatform(specPath *field.Path, spec hivev1.ClusterDeploymentSpec) field.ErrorList {
	allErrs := field.ErrorList{}
	canManageDNS := false
	if spec.Platform.AWS != nil {
		canManageDNS = true
	}
	if spec.Platform.Azure != nil {
		canManageDNS = true
	}
	if spec.Platform.GCP != nil {
		canManageDNS = true
	}
	if !canManageDNS && spec.ManageDNS {
		allErrs = append(allErrs, field.Invalid(specPath.Child("manageDNS"), spec.ManageDNS, "cannot manage DNS for the selected platform"))
	}
	return allErrs
}

// validateUpdate specifically validates update operations for ClusterDeployment objects.
func (a *ClusterDeploymentValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	if admResp := validatefeatureGates(a.decoder, admissionSpec, a.fs, contextLogger); admResp != nil {
		contextLogger.Errorf("object was rejected due to feature gate failures")
		return admResp
	}

	cd := &hivev1.ClusterDeployment{}
	if err := a.decoder.DecodeRaw(admissionSpec.Object, cd); err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	// Add the new data to the contextLogger
	contextLogger.Data["object.Name"] = cd.Name

	oldObject := &hivev1.ClusterDeployment{}
	if err := a.decoder.DecodeRaw(admissionSpec.OldObject, oldObject); err != nil {
		contextLogger.Errorf("Failed unmarshaling OldObject: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	// Add the new data to the contextLogger
	contextLogger.Data["oldObject.Name"] = oldObject.Name

	hasChangedImmutableField, unsupportedDiff := hasChangedImmutableField(&oldObject.Spec, &cd.Spec)
	if hasChangedImmutableField {
		message := fmt.Sprintf("Attempted to change ClusterDeployment.Spec which is immutable except for %s fields. Unsupported change: \n%s", strings.Join(mutableFields, ","), unsupportedDiff)
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// validate the newly incoming ingress
	if ingressValidationResult := validateIngress(cd, contextLogger); ingressValidationResult != nil {
		return ingressValidationResult
	}

	// Now catch the case where there was a previously defined list and now it's being emptied
	hasClearedOutPreviouslyDefinedIngressList := hasClearedOutPreviouslyDefinedIngressList(&oldObject.Spec, &cd.Spec)
	if hasClearedOutPreviouslyDefinedIngressList {
		message := "Previously defined a list of ingress objects, must provide a default ingress object"
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	if cd.Spec.Installed {
		if cd.Spec.ClusterMetadata != nil {
			if oldObject.Spec.Installed {
				if cd.Spec.ClusterMetadata.Platform != nil {
					// Special case: allow setting or changing -- but not unsetting -- Azure resource group name.
					if cd.Spec.ClusterMetadata.Platform.Azure != nil && cd.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName != nil {
						if oldObject.Spec.ClusterMetadata.Platform == nil {
							oldObject.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{}
						}
						if oldObject.Spec.ClusterMetadata.Platform.Azure == nil {
							oldObject.Spec.ClusterMetadata.Platform.Azure = &azure.Metadata{}
						}
						// copy over the value to spoof the immutability checker
						oldObject.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName = cd.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName
					}
					// Special case: allow setting or changing -- but not unsetting -- AWS hosted zone role.
					if cd.Spec.ClusterMetadata.Platform.AWS != nil && cd.Spec.ClusterMetadata.Platform.AWS.HostedZoneRole != nil {
						if oldObject.Spec.ClusterMetadata.Platform == nil {
							oldObject.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{}
						}
						if oldObject.Spec.ClusterMetadata.Platform.AWS == nil {
							oldObject.Spec.ClusterMetadata.Platform.AWS = &aws.Metadata{}
						}
						// copy over the value to spoof the immutability checker
						oldObject.Spec.ClusterMetadata.Platform.AWS.HostedZoneRole = cd.Spec.ClusterMetadata.Platform.AWS.HostedZoneRole
					}
					// Special case: allow setting or changing -- but not unsetting -- GCP Network Project ID.
					if cd.Spec.ClusterMetadata.Platform.GCP != nil && cd.Spec.ClusterMetadata.Platform.GCP.NetworkProjectID != nil {
						if oldObject.Spec.ClusterMetadata.Platform == nil {
							oldObject.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{}
						}
						if oldObject.Spec.ClusterMetadata.Platform.GCP == nil {
							oldObject.Spec.ClusterMetadata.Platform.GCP = &gcp.Metadata{}
						}
						// copy over the value to spoof the immutability checker
						oldObject.Spec.ClusterMetadata.Platform.GCP.NetworkProjectID = cd.Spec.ClusterMetadata.Platform.GCP.NetworkProjectID
					}
				}
				allErrs = append(allErrs, apivalidation.ValidateImmutableField(cd.Spec.ClusterMetadata, oldObject.Spec.ClusterMetadata, specPath.Child("clusterMetadata"))...)
			}
		} else {
			allErrs = append(allErrs, field.Required(specPath.Child("clusterMetadata"), "installed cluster must have cluster metadata"))
		}
	} else {
		if oldObject.Spec.Installed {
			allErrs = append(allErrs, field.Invalid(specPath.Child("installed"), cd.Spec.Installed, "cannot make uninstalled once installed"))
		}
	}

	// Validate the ClusterPoolRef:
	switch oldPoolRef, newPoolRef := oldObject.Spec.ClusterPoolRef, cd.Spec.ClusterPoolRef; {
	case oldPoolRef != nil && newPoolRef != nil:
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newPoolRef.Namespace, oldPoolRef.Namespace, specPath.Child("clusterPoolRef", "namespace"))...)
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newPoolRef.PoolName, oldPoolRef.PoolName, specPath.Child("clusterPoolRef", "poolName"))...)
		if oldClaim := oldPoolRef.ClaimName; oldClaim != "" {
			allErrs = append(allErrs, apivalidation.ValidateImmutableField(newPoolRef.ClaimName, oldClaim, specPath.Child("clusterPoolRef", "claimName"))...)
		}
	case oldPoolRef != nil && newPoolRef == nil:
		allErrs = append(allErrs, field.Invalid(specPath.Child("clusterPoolRef"), newPoolRef, "cannot remove clusterPoolRef"))
	case oldPoolRef == nil && newPoolRef != nil:
		allErrs = append(allErrs, field.Invalid(specPath.Child("clusterPoolRef"), newPoolRef, "cannot add clusterPoolRef"))
	}

	if len(allErrs) > 0 {
		contextLogger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(admissionSpec.Kind).GroupKind(), admissionSpec.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateDelete specifically validates delete operations for ClusterDeployment objects.
func (a *ClusterDeploymentValidatingAdmissionHook) validateDelete(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	logger := log.WithFields(log.Fields{
		"operation": request.Operation,
		"group":     request.Resource.Group,
		"version":   request.Resource.Version,
		"resource":  request.Resource.Resource,
		"method":    "validateDelete",
	})

	oldObject := &hivev1.ClusterDeployment{}
	if err := a.decoder.DecodeRaw(request.OldObject, oldObject); err != nil {
		logger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	logger.Data["object.Name"] = oldObject.Name

	var allErrs field.ErrorList

	if value, present := oldObject.Annotations[constants.ProtectedDeleteAnnotation]; present {
		if enabled, err := strconv.ParseBool(value); enabled && err == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("metadata", "annotations", constants.ProtectedDeleteAnnotation),
				oldObject.Annotations[constants.ProtectedDeleteAnnotation],
				"cannot delete while annotation is present",
			))
		} else {
			logger.WithField(constants.ProtectedDeleteAnnotation, value).Info("Protected Delete annotation present but not set to true")
		}
	}

	if len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// hasChangedImmutableField determines if a ClusterDeployment.spec immutable field was changed.
// it returns the diff string that shows the changes that are not supported
func hasChangedImmutableField(oldObject, cd *hivev1.ClusterDeploymentSpec) (bool, string) {
	r := &diffReporter{}
	opts := cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(hivev1.ClusterDeploymentSpec{}, mutableFields...),
		cmp.Reporter(r),
	}
	return !cmp.Equal(oldObject, cd, opts), r.String()
}

// diffReporter is a simple custom reporter that only records differences
// detected during comparison.
type diffReporter struct {
	path  cmp.Path
	diffs []string
}

func (r *diffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *diffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		p := r.path.String()
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("\t%s: (%+v => %+v)", p, vx, vy))
	}
}

func (r *diffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *diffReporter) String() string {
	return strings.Join(r.diffs, "\n")
}

func hasClearedOutPreviouslyDefinedIngressList(oldObject, cd *hivev1.ClusterDeploymentSpec) bool {
	// We don't allow a ClusterDeployment which had previously defined a list of Ingress objects
	// to then be cleared out. It either must be cleared from the beginning (ie just use default behavior),
	// or the ClusterDeployment must continue to define at least the 'default' ingress object.
	if len(oldObject.Ingress) > 0 && len(cd.Ingress) == 0 {
		return true
	}

	return false
}

func validateIngressDomainsShareClusterDomain(cd *hivev1.ClusterDeploymentSpec) bool {
	// ingress entries must share the same domain as the cluster
	// so watch for an ingress domain ending in: .<clusterName>.<baseDomain>
	regexString := fmt.Sprintf(`(?i).*\.%s.%s$`, cd.ClusterName, cd.BaseDomain)
	sharedSubdomain := regexp.MustCompile(regexString)

	for _, ingress := range cd.Ingress {
		if !sharedSubdomain.Match([]byte(ingress.Domain)) {
			return false
		}
	}
	return true
}
func validateIngressDomainsNotWildcard(cd *hivev1.ClusterDeploymentSpec) bool {
	// check for domains with leading '*'
	// the * is unnecessary as the ingress controller assumes a wildcard
	for _, ingress := range cd.Ingress {
		if ingress.Domain[0] == '*' {
			return false
		}
	}
	return true
}

func validateIngressServingCertificateExists(cd *hivev1.ClusterDeploymentSpec) bool {
	// Include the empty string in the set of certs so that an ingress with
	// an empty serving certificate passes.
	certs := sets.NewString("")
	for _, cert := range cd.CertificateBundles {
		certs.Insert(cert.Name)
	}
	for _, ingress := range cd.Ingress {
		if !certs.Has(ingress.ServingCertificate) {
			return false
		}
	}
	return true
}

// empty ingress is allowed (for create), but if it's non-zero
// it must include an entry for 'default'
func validateIngressList(cd *hivev1.ClusterDeploymentSpec) bool {
	if len(cd.Ingress) == 0 {
		return true
	}

	defaultFound := false
	for _, ingress := range cd.Ingress {
		if ingress.Name == "default" {
			defaultFound = true
		}
	}
	return defaultFound
}

func validateDomain(domain string, validDomains []string) bool {
	matchFound := false
	for _, validDomain := range validDomains {
		// Do not allow the base domain to be the same as one of the managed domains.
		if domain == validDomain {
			return false
		}
		dottedValidDomain := "." + validDomain
		if !strings.HasSuffix(domain, dottedValidDomain) {
			continue
		}
		childPart := strings.TrimSuffix(domain, dottedValidDomain)
		if !strings.ContainsRune(childPart, '.') {
			matchFound = true
		}
	}
	return matchFound
}

func validateIngress(cd *hivev1.ClusterDeployment, contextLogger *log.Entry) *admissionv1beta1.AdmissionResponse {
	if !validateIngressList(&cd.Spec) {
		message := "Ingress list must include a default entry"
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressDomainsNotWildcard(&cd.Spec) {
		message := "Ingress domains must not lead with *"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressDomainsShareClusterDomain(&cd.Spec) {
		message := "Ingress domains must share the same domain as the cluster"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressServingCertificateExists(&cd.Spec) {
		message := "Ingress has serving certificate that does not exist in certificate bundle"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// everything passed
	return nil
}

func validateCertificateBundles(cd *hivev1.ClusterDeployment, contextLogger *log.Entry) *admissionv1beta1.AdmissionResponse {
	for _, certBundle := range cd.Spec.CertificateBundles {
		if certBundle.Name == "" {
			message := "Certificate bundle is missing a name"
			contextLogger.Infof("Failed validation: %v", message)
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
		if certBundle.CertificateSecretRef.Name == "" {
			message := "Certificate bundle is missing a secret reference"
			contextLogger.Infof("Failed validation: %v", message)
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
	}
	return nil
}
