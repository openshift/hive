package v1

import (
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	"github.com/openshift/hive/pkg/constants"
)

const (
	machinePoolGroup    = "hive.openshift.io"
	machinePoolVersion  = "v1"
	machinePoolResource = "machinepools"

	defaultMasterPoolName = "master"
	defaultWorkerPoolName = "worker"
	legacyWorkerPoolName  = "w"
)

// MachinePoolValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type MachinePoolValidatingAdmissionHook struct {
	decoder admission.Decoder
}

// NewMachinePoolValidatingAdmissionHook constructs a new MachinePoolValidatingAdmissionHook
func NewMachinePoolValidatingAdmissionHook(decoder admission.Decoder) *MachinePoolValidatingAdmissionHook {
	return &MachinePoolValidatingAdmissionHook{decoder: decoder}
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/machinePoolvalidators".
// When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *MachinePoolValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "machinepoolvalidator",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the MachinePool CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1",
			Resource: "machinepoolvalidators",
		},
		"machinepoolvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *MachinePoolValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "machinepoolvalidator",
	}).Info("Initializing validation REST resource")

	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *MachinePoolValidatingAdmissionHook) Validate(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	logger := log.WithFields(log.Fields{
		"operation": request.Operation,
		"group":     request.Resource.Group,
		"version":   request.Resource.Version,
		"resource":  request.Resource.Resource,
		"method":    "Validate",
	})

	if !a.shouldValidate(request, logger) {
		logger.Info("Skipping validation for request")
		// The request object isn't something that this validator should validate.
		// Therefore, we say that it's allowed.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	logger.Info("Validating request")

	switch request.Operation {
	case admissionv1beta1.Create:
		return a.validateCreateRequest(request, logger)
	case admissionv1beta1.Update:
		return a.validateUpdateRequest(request, logger)
	default:
		logger.Info("Successful validation")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}

// shouldValidate explicitly checks if the request should validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *MachinePoolValidatingAdmissionHook) shouldValidate(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) bool {
	logger = logger.WithField("method", "shouldValidate")

	if request.Resource.Group != machinePoolGroup {
		logger.Debug("Returning False, not our group")
		return false
	}

	if request.Resource.Version != machinePoolVersion {
		logger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if request.Resource.Resource != machinePoolResource {
		logger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	logger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreateRequest specifically validates create operations for MachinePool objects.
func (a *MachinePoolValidatingAdmissionHook) validateCreateRequest(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) *admissionv1beta1.AdmissionResponse {
	logger = logger.WithField("method", "validateCreateRequest")

	newObject, resp := a.decode(request.Object, logger.WithField("decode", "Object"))
	if resp != nil {
		return resp
	}

	logger = logger.
		WithField("object.Name", newObject.Name).
		WithField("object.Namespace", newObject.Namespace)

	if allErrs := validateMachinePoolCreate(newObject); len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateUpdateRequest specifically validates update operations for MachinePool objects.
func (a *MachinePoolValidatingAdmissionHook) validateUpdateRequest(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) *admissionv1beta1.AdmissionResponse {
	logger = logger.WithField("method", "validateUpdateRequest")

	newObject, resp := a.decode(request.Object, logger.WithField("decode", "Object"))
	if resp != nil {
		return resp
	}

	logger = logger.
		WithField("object.Name", newObject.Name).
		WithField("object.Namespace", newObject.Namespace)

	oldObject, resp := a.decode(request.OldObject, logger.WithField("decode", "OldObject"))
	if resp != nil {
		return resp
	}

	if allErrs := validateMachinePoolUpdate(oldObject, newObject); len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

func (a *MachinePoolValidatingAdmissionHook) decode(raw runtime.RawExtension, logger log.FieldLogger) (*hivev1.MachinePool, *admissionv1beta1.AdmissionResponse) {
	obj := &hivev1.MachinePool{}
	if err := a.decoder.DecodeRaw(raw, obj); err != nil {
		logger.WithError(err).Error("failed to decode")
		return nil, &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}
	return obj, nil
}

func validateMachinePoolCreate(pool *hivev1.MachinePool) field.ErrorList {
	return validateMachinePoolInvariants(pool)
}

func validateMachinePoolUpdate(old, new *hivev1.MachinePool) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateMachinePoolInvariants(new)...)
	specPath := field.NewPath("spec")
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.ClusterDeploymentRef, old.Spec.ClusterDeploymentRef, specPath.Child("clusterDeploymentRef"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.Name, old.Spec.Name, specPath.Child("name"))...)
	if mutable, err := strconv.ParseBool(new.Annotations[constants.OverrideMachinePoolPlatformAnnotation]); err != nil || !mutable {
		allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.Platform, old.Spec.Platform, specPath.Child("platform"))...)
	}
	return allErrs
}

func validateMachinePoolName(pool *hivev1.MachinePool) field.ErrorList {
	allErrs := field.ErrorList{}
	if pool.Name != fmt.Sprintf("%s-%s", pool.Spec.ClusterDeploymentRef.Name, pool.Spec.Name) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metadata", "name"), pool.Name, "name must be ${CD_NAME}-${POOL_NAME}, where ${CD_NAME} is the name of the clusterdeployment and ${POOL_NAME} is the name of the remote machine pool"))
	}
	for _, invalidName := range []string{defaultMasterPoolName, legacyWorkerPoolName} {
		if pool.Spec.Name == invalidName {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "name"), pool.Spec.Name, fmt.Sprintf("pool name cannot be %q", invalidName)))
		}
	}
	return allErrs
}

func validateMachinePoolInvariants(pool *hivev1.MachinePool) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateMachinePoolName(pool)...)
	allErrs = append(allErrs, validateMachinePoolSpecInvariants(&pool.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateMachinePoolSpecInvariants(spec *hivev1.MachinePoolSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if spec.ClusterDeploymentRef.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("clusterDeploymentRef", "name"), "must have reference to clusterdeployment"))
	}
	if spec.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "must have a name for the remote machine pool"))
	}
	if spec.Replicas != nil {
		if spec.Autoscaling != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("replicas"), *spec.Replicas, "replicas must not be specified when autoscaling is specified"))
		}
		if *spec.Replicas < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("replicas"), *spec.Replicas, "replicas count must not be negative"))
		}
	}
	platformPath := fldPath.Child("platform")
	platforms := []string{}
	numberOfMachineSets := 0

	// set validZeroSizeAutoscalingMinReplicas to true for any platform where a zero-size minReplicas is allowed with autoscaling
	validZeroSizeAutoscalingMinReplicas := false

	if p := spec.Platform.AWS; p != nil {
		platforms = append(platforms, "aws")
		allErrs = append(allErrs, validateAWSMachinePoolPlatformInvariants(p, platformPath.Child("aws"))...)
		numberOfMachineSets = len(p.Zones)
		validZeroSizeAutoscalingMinReplicas = true
	}
	if p := spec.Platform.Azure; p != nil {
		platforms = append(platforms, "azure")
		allErrs = append(allErrs, validateAzureMachinePoolPlatformInvariants(p, platformPath.Child("azure"))...)
		numberOfMachineSets = len(p.Zones)
		validZeroSizeAutoscalingMinReplicas = true
	}
	if p := spec.Platform.GCP; p != nil {
		platforms = append(platforms, "gcp")
		allErrs = append(allErrs, validateGCPMachinePoolPlatformInvariants(p, platformPath.Child("gcp"))...)
		numberOfMachineSets = len(p.Zones)
		validZeroSizeAutoscalingMinReplicas = true
	}
	if p := spec.Platform.OpenStack; p != nil {
		platforms = append(platforms, "openstack")
		allErrs = append(allErrs, validateOpenStackMachinePoolPlatformInvariants(p, platformPath.Child("openstack"))...)
		validZeroSizeAutoscalingMinReplicas = true
	}
	if p := spec.Platform.VSphere; p != nil {
		platforms = append(platforms, "vsphere")
		allErrs = append(allErrs, validateVSphereMachinePoolPlatformInvariants(p, platformPath.Child("vsphere"))...)
	}
	if p := spec.Platform.IBMCloud; p != nil {
		platforms = append(platforms, "ibmcloud")
		allErrs = append(allErrs, validateIBMCloudMachinePoolPlatformInvariants(p, platformPath.Child("ibmcloud"))...)
	}
	if p := spec.Platform.Nutanix; p != nil {
		platforms = append(platforms, "nutanix")
		allErrs = append(allErrs, validateNutanixMachinePoolPlatformInvariants(p, platformPath.Child("nutanix"))...)
	}

	switch len(platforms) {
	case 0:
		allErrs = append(allErrs, field.Required(platformPath, "must specify a platform"))
	case 1:
		// valid
	default:
		allErrs = append(allErrs, field.Invalid(platformPath, spec.Platform, fmt.Sprintf("multiple platforms specified: %s", platforms)))
	}
	if spec.Autoscaling != nil {
		autoscalingPath := fldPath.Child("autoscaling")
		if numberOfMachineSets == 0 {
			if spec.Autoscaling.MinReplicas < 1 && !validZeroSizeAutoscalingMinReplicas {
				allErrs = append(allErrs, field.Invalid(autoscalingPath.Child("minReplicas"), spec.Autoscaling.MinReplicas, "minimum replicas must be at least 1"))
			}
		} else {
			if spec.Autoscaling.MinReplicas < int32(numberOfMachineSets) && !validZeroSizeAutoscalingMinReplicas {
				allErrs = append(allErrs, field.Invalid(autoscalingPath.Child("minReplicas"), spec.Autoscaling.MinReplicas, "minimum replicas must be at least the number of zones"))
			}
		}
		if spec.Autoscaling.MinReplicas > spec.Autoscaling.MaxReplicas {
			allErrs = append(allErrs, field.Invalid(autoscalingPath.Child("minReplicas"), spec.Autoscaling.MinReplicas, "minimum replicas must not be greater than maximum replicas"))
		}
	}
	allErrs = append(allErrs, metavalidation.ValidateLabels(spec.Labels, fldPath.Child("labels"))...)
	return allErrs
}

func validateAWSMachinePoolPlatformInvariants(platform *hivev1aws.MachinePoolPlatform, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, zone := range platform.Zones {
		if zone == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("zones").Index(i), zone, "zone cannot be an empty string"))
		}
	}
	if platform.InstanceType == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instance type is required"))
	}
	rootVolume := &platform.EC2RootVolume
	rootVolumePath := fldPath.Child("ec2RootVolume")
	if rootVolume.IOPS < 0 {
		allErrs = append(allErrs, field.Invalid(rootVolumePath.Child("iops"), rootVolume.IOPS, "volume IOPS must not be negative"))
	}
	if rootVolume.Size < 0 {
		allErrs = append(allErrs, field.Invalid(rootVolumePath.Child("size"), rootVolume.Size, "volume size must not be negative"))
	}
	if rootVolume.Type == "" {
		allErrs = append(allErrs, field.Required(rootVolumePath.Child("type"), "volume type is required"))
	}
	return allErrs
}

func validateGCPMachinePoolPlatformInvariants(platform *hivev1gcp.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, zone := range platform.Zones {
		if zone == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("zones").Index(i), zone, "zone cannot be an empty string"))
		}
	}
	if platform.InstanceType == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instance type is required"))
	}
	return allErrs
}

func validateAzureMachinePoolPlatformInvariants(platform *hivev1azure.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, zone := range platform.Zones {
		if zone == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("zones").Index(i), zone, "zone cannot be an empty string"))
		}
	}
	if platform.InstanceType == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instance type is required"))
	}
	osDisk := &platform.OSDisk
	osDiskPath := fldPath.Child("osDisk")
	if osDisk.DiskSizeGB <= 0 {
		allErrs = append(allErrs, field.Invalid(osDiskPath.Child("iops"), osDisk.DiskSizeGB, "disk size must be positive"))
	}
	return allErrs
}

func validateOpenStackMachinePoolPlatformInvariants(platform *hivev1openstack.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if platform.Flavor == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("type"), "flavor name is required"))
	}
	return allErrs
}

func validateVSphereMachinePoolPlatformInvariants(platform *hivev1vsphere.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if platform.NumCPUs <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("numCPUs"), "number of cpus must be positive"))
	}

	if platform.NumCoresPerSocket <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("numCoresPerSocket"), "number of cores per socket must be positive"))
	}

	if platform.MemoryMiB <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("memoryMiB"), "memory must be positive"))
	}

	if platform.OSDisk.DiskSizeGB <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("diskSizeGB"), "disk size must be positive"))
	}

	return allErrs
}

func validateIBMCloudMachinePoolPlatformInvariants(platform *hivev1ibmcloud.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	return allErrs
}

func validateNutanixMachinePoolPlatformInvariants(platform *hivev1nutanix.MachinePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if platform.NumCPUs <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("numCPUs"), "number of vCPUs must be positive"))
	}

	if platform.NumCoresPerSocket <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("numCoresPerSocket"), "number of cores per sockets must be positive"))
	}

	if platform.MemoryMiB <= 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("memoryMiB"), "memory must be positive"))
	}

	if len(platform.DataDisks) > 0 {
		for _, disk := range platform.DataDisks {
			if disk.DiskSize.IsZero() {
				allErrs = append(allErrs, field.Required(fldPath.Child("diskSizeBytes"), "disk size must be set"))
			}
		}
	}

	return allErrs
}
