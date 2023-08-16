package vsphere

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	apimachineryutilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	machinev1 "github.com/openshift/api/machine/v1beta1"

	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	"github.com/openshift/machine-api-operator/pkg/controller/vsphere/session"
	"github.com/openshift/machine-api-operator/pkg/metrics"
)

const (
	fullCloneDiskMoveType = string(types.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndConsolidate)
	linkCloneDiskMoveType = string(types.VirtualMachineRelocateDiskMoveOptionsCreateNewChildDiskBacking)
	ethCardType           = "vmxnet3"
	providerIDPrefix      = "vsphere://"
	regionKey             = "region"
	zoneKey               = "zone"
	minimumHWVersion      = 15
)

// These are the guestinfo variables used by Ignition.
// https://access.redhat.com/documentation/en-us/openshift_container_platform/4.1/html/installing/installing-on-vsphere
const (
	GuestInfoIgnitionData     = "guestinfo.ignition.config.data"
	GuestInfoIgnitionEncoding = "guestinfo.ignition.config.data.encoding"
	GuestInfoHostname         = "guestinfo.hostname"
	StealClock                = "stealclock.enable"
)

// vSphere tasks description IDs, for determinate task types (clone, delete, etc)
const (
	cloneVmTaskDescriptionId    = "VirtualMachine.clone"
	destroyVmTaskDescriptionId  = "VirtualMachine.destroy"
	powerOffVmTaskDescriptionId = "VirtualMachine.powerOff"
)

// Reconciler runs the logic to reconciles a machine resource towards its desired state
type Reconciler struct {
	*machineScope
}

func newReconciler(scope *machineScope) *Reconciler {
	return &Reconciler{
		machineScope: scope,
	}
}

// create creates machine if it does not exists.
func (r *Reconciler) create() error {
	if err := validateMachine(*r.machine); err != nil {
		return fmt.Errorf("%v: failed validating machine provider spec: %w", r.machine.GetName(), err)
	}

	// We only clone the VM template if we have no taskRef.
	if r.providerStatus.TaskRef == "" {
		if !r.machineScope.session.IsVC() {
			return fmt.Errorf("%v: not connected to a vCenter", r.machine.GetName())
		}
		klog.Infof("%v: cloning", r.machine.GetName())
		task, err := clone(r.machineScope)
		if err != nil {
			metrics.RegisterFailedInstanceCreate(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    "Clone task finished with error",
			})
			conditionFailed := conditionFailed()
			conditionFailed.Message = err.Error()
			statusError := setProviderStatus(task, conditionFailed, r.machineScope, nil)
			if statusError != nil {
				return fmt.Errorf("failed to set provider status: %w", err)
			}
			return err
		}
		return setProviderStatus(task, conditionSuccess(), r.machineScope, nil)
	}

	moTask, err := r.session.GetTask(r.Context, r.providerStatus.TaskRef)
	if err != nil {
		metrics.RegisterFailedInstanceCreate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    "GetTask finished with error",
		})
		return err
	}

	if moTask == nil {
		// Possible eventual consistency problem from vsphere
		// TODO: change error message here to indicate this might be expected.
		return fmt.Errorf("unexpected moTask nil")
	}

	if taskIsFinished, err := taskIsFinished(moTask); err != nil {
		if taskIsFinished {
			metrics.RegisterFailedInstanceCreate(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    "Task finished with error",
			})
			conditionFailed := conditionFailed()
			conditionFailed.Message = err.Error()
			statusError := setProviderStatus(moTask.Reference().Value, conditionFailed, r.machineScope, nil)
			if statusError != nil {
				return fmt.Errorf("failed to set provider status: %w", statusError)
			}
			return machinecontroller.CreateMachine(err.Error())
		} else {
			return fmt.Errorf("failed to check task status: %w", err)
		}
	} else if !taskIsFinished {
		return fmt.Errorf("%v task %v has not finished", moTask.Info.DescriptionId, moTask.Reference().Value)
	}

	// if clone task finished successfully, power on the vm
	if moTask.Info.DescriptionId == cloneVmTaskDescriptionId {
		klog.Infof("Powering on cloned machine: %v", r.machine.Name)
		task, err := powerOn(r.machineScope)
		if err != nil {
			metrics.RegisterFailedInstanceCreate(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    "PowerOn task finished with error",
			})
			conditionFailed := conditionFailed()
			conditionFailed.Message = err.Error()
			statusError := setProviderStatus(task, conditionFailed, r.machineScope, nil)
			if statusError != nil {
				return fmt.Errorf("failed to set provider status: %w", err)
			}
			return err
		}
		return setProviderStatus(task, conditionSuccess(), r.machineScope, nil)
	}

	// If taskIsFinished then next reconcile should result in update.
	return nil
}

// update finds a vm and reconciles the machine resource status against it.
func (r *Reconciler) update() error {
	if err := validateMachine(*r.machine); err != nil {
		return fmt.Errorf("%v: failed validating machine provider spec: %w", r.machine.GetName(), err)
	}

	if r.providerStatus.TaskRef != "" {
		moTask, err := r.session.GetTask(r.Context, r.providerStatus.TaskRef)
		if err != nil {
			if !isRetrieveMONotFound(r.providerStatus.TaskRef, err) {
				metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
					Name:      r.machine.Name,
					Namespace: r.machine.Namespace,
					Reason:    "GetTask finished with error",
				})
				return err
			}
		}
		if moTask != nil {
			if taskIsFinished, err := taskIsFinished(moTask); err != nil {
				metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
					Name:      r.machine.Name,
					Namespace: r.machine.Namespace,
					Reason:    "Task finished with error",
				})
				return fmt.Errorf("%v task %v finished with error: %w", moTask.Info.DescriptionId, moTask.Reference().Value, err)
			} else if !taskIsFinished {
				return fmt.Errorf("%v task %v has not finished", moTask.Info.DescriptionId, moTask.Reference().Value)
			}
		}
	}

	vmRef, err := findVM(r.machineScope)
	if err != nil {
		metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    "FindVM finished with error",
		})
		if !isNotFound(err) {
			return err
		}
		return fmt.Errorf("vm not found on update: %w", err)
	}

	vm := &virtualMachine{
		Context: r.machineScope.Context,
		Obj:     object.NewVirtualMachine(r.machineScope.session.Client.Client, vmRef),
		Ref:     vmRef,
	}

	if err := vm.reconcileTags(r.Context, r.session, r.machine); err != nil {
		metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    "ReconcileTags finished with error",
		})
		return fmt.Errorf("failed to reconcile tags: %w", err)
	}

	if err := r.reconcileMachineWithCloudState(vm, r.providerStatus.TaskRef); err != nil {
		metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    "ReconcileWithCloudState finished with error",
		})
		return err
	}

	return nil
}

// exists returns true if machine exists.
func (r *Reconciler) exists() (bool, error) {
	if err := validateMachine(*r.machine); err != nil {
		return false, fmt.Errorf("%v: failed validating machine provider spec: %w", r.machine.GetName(), err)
	}

	vmRef, err := findVM(r.machineScope)
	if err != nil {
		if !isNotFound(err) {
			return false, err
		}
		klog.Infof("%v: does not exist", r.machine.GetName())
		return false, nil
	}

	// Check if machine was powered on after clone.
	// If it is powered off and in "Provisioning" phase, treat machine as non-existed yet and requeue for proceed
	// with creation procedure.
	powerState := types.VirtualMachinePowerState(pointer.StringDeref(r.machineScope.providerStatus.InstanceState, ""))
	if powerState == "" {
		vm := &virtualMachine{
			Context: r.machineScope.Context,
			Obj:     object.NewVirtualMachine(r.machineScope.session.Client.Client, vmRef),
			Ref:     vmRef,
		}
		powerState, err = vm.getPowerState()
		if err != nil {
			return false, fmt.Errorf("%v: failed checking machine's power state: %w", r.machine.GetName(), err)
		}
	}

	if pointer.StringDeref(r.machine.Status.Phase, "") == machinev1.PhaseProvisioning && powerState == types.VirtualMachinePowerStatePoweredOff {
		klog.Infof("%v: already exists, but was not powered on after clone, requeue ", r.machine.GetName())
		return false, nil
	}

	klog.Infof("%v: already exists", r.machine.GetName())
	return true, nil
}

func (r *Reconciler) delete() error {
	if r.providerStatus.TaskRef != "" {
		// TODO: We need to use a separate status field for the create and the
		// delete taskref.
		moTask, err := r.session.GetTask(r.Context, r.providerStatus.TaskRef)
		if err != nil {
			if !isRetrieveMONotFound(r.providerStatus.TaskRef, err) {
				return err
			}
		}
		if moTask != nil {
			if taskIsFinished, err := taskIsFinished(moTask); err != nil {
				// Check if latest task is not a task for vm cloning
				if taskIsFinished && moTask.Info.DescriptionId != cloneVmTaskDescriptionId {
					metrics.RegisterFailedInstanceDelete(&metrics.MachineLabels{
						Name:      r.machine.Name,
						Namespace: r.machine.Namespace,
						Reason:    "Task finished with error",
					})
					klog.Errorf("Delete task finished with error: %w", err)
					return fmt.Errorf("%v task %v finished with error: %w", moTask.Info.DescriptionId, moTask.Reference().Value, err)
				} else {
					klog.Warningf(
						"TaskRef points to clone task which finished with error: %w. Proceeding with machine deletion", err,
					)
				}
			} else if !taskIsFinished {
				return fmt.Errorf("%v task %v has not finished", moTask.Info.DescriptionId, moTask.Reference().Value)
			}
		}
	}

	vmRef, err := findVM(r.machineScope)
	if err != nil {
		if !isNotFound(err) {
			metrics.RegisterFailedInstanceDelete(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    "FindVM finished with error",
			})
			return err
		}
		klog.Infof("%v: vm does not exist", r.machine.GetName())
		return nil
	}

	vm := &virtualMachine{
		Context: r.Context,
		Obj:     object.NewVirtualMachine(r.machineScope.session.Client.Client, vmRef),
		Ref:     vmRef,
	}

	powerState, err := vm.getPowerState()
	if err != nil {
		return fmt.Errorf("can not determine %v vm power state: %w", r.machine.GetName(), err)
	}
	if powerState != types.VirtualMachinePowerStatePoweredOff {
		powerOffTaskRef, err := vm.powerOffVM()
		if err != nil {
			return fmt.Errorf("%v: failed to power off vm: %w", r.machine.GetName(), err)
		}
		if err := setProviderStatus(powerOffTaskRef, conditionSuccess(), r.machineScope, vm); err != nil {
			return fmt.Errorf("failed to set provider status: %w", err)
		}
		return fmt.Errorf("powering off vm is in progress, requeuing")
	}

	// At this point node should be drained and vm powered off already.
	// We need to check attached disks and ensure that all disks potentially related to PVs were detached
	// to prevent possible data loss.
	// Destroying a VM with attached disks might lead to data loss in case pvs are handled by the intree storage driver.

	_, drainSkipped := r.machine.ObjectMeta.Annotations[machinecontroller.ExcludeNodeDrainingAnnotation]

	// If node linked to the machine, and node was drained checking node status first
	if r.machineScope.isNodeLinked() && !drainSkipped {
		// After node draining, make sure volumes are detached before deleting the Node.
		attached, err := r.nodeHasVolumesAttached(r.Context, r.machine.Status.NodeRef.Name, r.machine.Name)
		if err != nil {
			return fmt.Errorf("failed to determine if node %v has attached volumes: %w", r.machine.Status.NodeRef.Name, err)
		}
		if attached {
			// If there are volumes still attached, it's possible that node draining did not fully finish,
			// this might happen if the kubelet was non-functional during the draining procedure.
			// Try forcefully deleting pods in the "Terminating" state to trigger persistent volumes detachment.
			klog.Warningf(
				"Attached volumes detected on a powered off node, node draining may not succeed. " +
					"Attempting to delete unevicted pods",
			)
			numPodsDeleted, err := r.machineScope.deleteUnevictedPods()
			klog.Warningf("Deleted %d pods", numPodsDeleted)
			if err != nil {
				return fmt.Errorf("unable to fully drain node, can not delete unevicted pods: %w", err)
			}
			return fmt.Errorf("node %v has attached volumes, requeuing", r.machine.Status.NodeRef.Name)
		}
	}

	klog.V(3).Infof("Checking attached disks before vm destroy")
	disks, err := vm.getAttachedDisks()
	if err != nil {
		return fmt.Errorf("%v: can not obtain virtual disks attached to the vm: %w", r.machine.GetName(), err)
	}
	// Currently, MAPI does not provide any API knobs to configure additional volumes for a VM.
	// So, we are expecting the VM to have only one disk, which is OS disk.
	if len(disks) > 1 {
		// If node drain was skipped we need to detach disks forcefully to prevent possible data corruption.
		if drainSkipped {
			klog.V(1).Infof(
				"%s: drain was skipped for the machine, detaching disks before vm destruction to prevent data loss",
				r.machine.GetName(),
			)
			if err := vm.detachDisks(filterOutVmOsDisk(disks, r.machine)); err != nil {
				return fmt.Errorf("failed to detach disks: %w", err)
			}
			klog.V(1).Infof(
				"%s: disks were detached", r.machine.GetName(),
			)
			return errors.New(
				"disks were detached, vm will be attempted to destroy in next reconciliation, requeuing",
			)
		}

		// Block vm destruction till attach-detach controller has properly detached disks
		return errors.New(
			"additional attached disks detected, block vm destruction and wait for disks to be detached",
		)
	}

	task, err := vm.Obj.Destroy(r.Context)
	if err != nil {
		metrics.RegisterFailedInstanceDelete(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    "Destroy finished with error",
		})
		return fmt.Errorf("%v: failed to destroy vm: %w", r.machine.GetName(), err)
	}

	if err := setProviderStatus(task.Reference().Value, conditionSuccess(), r.machineScope, vm); err != nil {
		return fmt.Errorf("failed to set provider status: %w", err)
	}

	// TODO: consider returning an error to specify retry time here
	return fmt.Errorf("destroying vm in progress, requeuing")
}

// nodeHasVolumesAttached returns true if node status still have volumes attached
// pod deletion and volume detach happen asynchronously, so pod could be deleted before volume detached from the node
// this could cause issue for some storage provisioner, for example, vsphere-volume this is problematic
// because if the node is deleted before detach success, then the underline VMDK will be deleted together with the Machine
// so after node draining we need to check if all volumes are detached before deleting the node.
func (r *Reconciler) nodeHasVolumesAttached(ctx context.Context, nodeName string, machineName string) (bool, error) {
	node := &corev1.Node{}
	if err := r.apiReader.Get(ctx, apimachinerytypes.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Errorf("Could not find node from noderef, it may have already been deleted: %w", err)
			return false, nil
		}
		return true, err
	}

	return len(node.Status.VolumesAttached) != 0, nil
}

// reconcileMachineWithCloudState reconcile machineSpec and status with the latest cloud state
func (r *Reconciler) reconcileMachineWithCloudState(vm *virtualMachine, taskRef string) error {
	klog.V(3).Infof("%v: reconciling machine with cloud state", r.machine.GetName())
	// TODO: reconcile task

	if err := r.reconcileRegionAndZoneLabels(vm); err != nil {
		// Not treating this is as a fatal error for now.
		klog.Errorf("Failed to reconcile region and zone labels: %v", err)
	}

	klog.V(3).Infof("%v: reconciling providerID", r.machine.GetName())
	if err := r.reconcileProviderID(vm); err != nil {
		return err
	}

	klog.V(3).Infof("%v: reconciling network", r.machine.GetName())
	if err := r.reconcileNetwork(vm); err != nil {
		return err
	}

	klog.V(3).Infof("%v: reconciling powerstate annotation", r.machine.GetName())
	if err := r.reconcilePowerStateAnnontation(vm); err != nil {
		return err
	}

	return setProviderStatus(taskRef, conditionSuccess(), r.machineScope, vm)
}

// reconcileRegionAndZoneLabels reconciles the labels on the Machine containing
// region and zone information -- provided the vSphere cloud provider has been
// configured with the labels that identify region and zone, and the configured
// tags are found somewhere in the ancestry of the given virtual machine.
func (r *Reconciler) reconcileRegionAndZoneLabels(vm *virtualMachine) error {
	if r.vSphereConfig == nil {
		klog.Warning("No vSphere cloud provider config. " +
			"Will not set region and zone labels.")
		return nil
	}

	regionLabel := r.vSphereConfig.Labels.Region
	zoneLabel := r.vSphereConfig.Labels.Zone

	var res map[string]string

	err := r.session.WithCachingTagsManager(vm.Context, func(c *session.CachingTagsManager) error {
		var err error
		res, err = vm.getRegionAndZone(c, regionLabel, zoneLabel)

		return err
	})

	if err != nil {
		return err
	}

	if r.machine.Labels == nil {
		r.machine.Labels = make(map[string]string)
	}

	r.machine.Labels[machinecontroller.MachineRegionLabelName] = res[regionKey]
	r.machine.Labels[machinecontroller.MachineAZLabelName] = res[zoneKey]

	return nil
}

func (r *Reconciler) reconcileProviderID(vm *virtualMachine) error {
	providerID, err := convertUUIDToProviderID(vm.Obj.UUID(vm.Context))
	if err != nil {
		return err
	}
	r.machine.Spec.ProviderID = &providerID
	return nil
}

// convertUUIDToProviderID transforms a UUID string into a provider ID.
func convertUUIDToProviderID(UUID string) (string, error) {
	parsedUUID, err := uuid.Parse(UUID)
	if err != nil {
		return "", err
	}
	return providerIDPrefix + parsedUUID.String(), nil
}

func (r *Reconciler) reconcileNetwork(vm *virtualMachine) error {
	currentNetworkStatusList, err := vm.getNetworkStatusList(r.session.Client.Client)
	if err != nil {
		return fmt.Errorf("error getting network status: %v", err)
	}

	//If the VM is powered on then issue requeues until all of the VM's
	//networks have IP addresses.
	expectNetworkLen, currentNetworkLen := len(r.providerSpec.Network.Devices), len(currentNetworkStatusList)
	if expectNetworkLen != currentNetworkLen {
		return fmt.Errorf("invalid network count: expected=%d current=%d", expectNetworkLen, currentNetworkLen)
	}

	var ipAddrs []corev1.NodeAddress
	for _, netStatus := range currentNetworkStatusList {
		for _, ip := range netStatus.IPAddrs {
			ipAddrs = append(ipAddrs, corev1.NodeAddress{
				Type:    corev1.NodeInternalIP,
				Address: ip,
			})
		}
	}

	// Using Name() if InventoryPath is empty will return empty name
	// see: https://github.com/vmware/govmomi/blob/master/object/common.go#L66-L75
	// Using ObjectName() as it will query from VirtualMachine properties

	vmName, err := vm.Obj.ObjectName(vm.Context)
	if err != nil {
		return fmt.Errorf("error getting virtual machine name: %v", err)
	}

	ipAddrs = append(ipAddrs, corev1.NodeAddress{
		Type:    corev1.NodeInternalDNS,
		Address: vmName,
	})

	klog.V(3).Infof("%v: reconciling network: IP addresses: %v", r.machine.GetName(), ipAddrs)
	r.machine.Status.Addresses = ipAddrs
	return nil
}

func (r *Reconciler) reconcilePowerStateAnnontation(vm *virtualMachine) error {
	if vm == nil {
		return errors.New("provided VM is nil")
	}

	// This can return an error if machine is being deleted
	powerState, err := vm.getPowerState()
	if err != nil {
		return err
	}

	if r.machine.Annotations == nil {
		r.machine.Annotations = map[string]string{}
	}
	r.machine.Annotations[machinecontroller.MachineInstanceStateAnnotationName] = string(powerState)

	return nil
}

func validateMachine(machine machinev1.Machine) error {
	if machine.Labels[machinev1.MachineClusterIDLabel] == "" {
		return machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1.MachineClusterIDLabel)
	}

	return nil
}

func findVM(s *machineScope) (types.ManagedObjectReference, error) {
	uuid := string(s.machine.UID)

	vm, err := s.GetSession().FindVM(s.Context, uuid, s.machine.Name)
	if err != nil {
		if isNotFound(err) {
			return types.ManagedObjectReference{}, errNotFound{instanceUUID: true, uuid: uuid}
		}
		return types.ManagedObjectReference{}, err
	}

	if vm == nil {
		return types.ManagedObjectReference{}, errNotFound{instanceUUID: true, uuid: uuid}
	}

	return vm.Reference(), nil
}

// errNotFound is returned by the findVM function when a VM is not found.
type errNotFound struct {
	instanceUUID bool
	uuid         string
}

func (e errNotFound) Error() string {
	if e.instanceUUID {
		return fmt.Sprintf("vm with instance uuid %s not found", e.uuid)
	}
	return fmt.Sprintf("vm with bios uuid %s not found", e.uuid)
}

func isNotFound(err error) bool {
	switch err.(type) {
	case errNotFound, *errNotFound, *find.NotFoundError:
		return true
	default:
		return false
	}
}

func isRetrieveMONotFound(taskRef string, err error) bool {
	return err.Error() == fmt.Sprintf("ServerFaultCode: The object 'vim.Task:%v' has already been deleted or has not been completely created", taskRef)
}

func getHwVersion(ctx context.Context, vm *object.VirtualMachine) (int, error) {
	var _vm mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"config.version"}, &_vm); err != nil {
		return 0, fmt.Errorf("error getting hw version information for vm %s: %w", vm.Name(), err)
	}

	versionString := _vm.Config.Version
	version := strings.TrimPrefix(versionString, "vmx-")
	parsedVersion, err := strconv.Atoi(version)
	if err != nil {
		return 0, fmt.Errorf("can not extract hardware version from version string: %s, format unknown", versionString)
	}
	return parsedVersion, nil
}

func clone(s *machineScope) (string, error) {
	userData, err := s.GetUserData()
	if err != nil {
		return "", err
	}

	vmTemplate, err := s.GetSession().FindVM(*s, "", s.providerSpec.Template)
	if err != nil {
		const multipleFoundMsg = "multiple templates found, specify one in config"
		const notFoundMsg = "template not found, specify valid value"
		defaultError := fmt.Errorf("unable to get template %q: %w", s.providerSpec.Template, err)
		return "", handleVSphereError(multipleFoundMsg, notFoundMsg, defaultError, err)
	}

	hwVersion, err := getHwVersion(s.Context, vmTemplate)
	if err != nil {
		return "", machinecontroller.InvalidMachineConfiguration(
			"Unable to detect machine template HW version for machine '%s': %v", s.machine.GetName(), err,
		)
	}
	if hwVersion < minimumHWVersion {
		return "", machinecontroller.InvalidMachineConfiguration(
			fmt.Sprintf(
				"Hardware lower than %d is not supported, clone stopped. "+
					"Detected machine template version is %d. "+
					"Please update machine template: https://access.redhat.com/articles/6090681",
				minimumHWVersion, hwVersion,
			),
		)
	}

	// Default clone type is FullClone, having snapshot on clonee template will cause incorrect disk sizing.
	diskMoveType := fullCloneDiskMoveType
	var snapshotRef *types.ManagedObjectReference

	// If a linked clone is requested then a MoRef for a snapshot must be
	// found with which to perform the linked clone.
	// Empty clone mode is a full clone,
	// because otherwise disk size from provider spec will not be respected.
	if s.providerSpec.CloneMode == machinev1.LinkedClone {
		if s.providerSpec.DiskGiB > 0 {
			klog.Warningf("LinkedClone mode is set. Disk size parameter from ProviderSpec will be ignored")
		}
		if s.providerSpec.Snapshot == "" {
			klog.V(3).Infof("%v: no snapshot name provided, getting snapshot using template", s.machine.GetName())
			var vm mo.VirtualMachine
			if err := vmTemplate.Properties(s.Context, vmTemplate.Reference(), []string{"snapshot"}, &vm); err != nil {
				return "", fmt.Errorf("error getting snapshot information for template %s: %w", vmTemplate.Name(), err)
			}

			if vm.Snapshot != nil {
				snapshotRef = vm.Snapshot.CurrentSnapshot
			}
		} else {
			klog.V(3).Infof("%v: searching for snapshot by name %s", s.machine.GetName(), s.providerSpec.Snapshot)
			var err error
			snapshotRef, err = vmTemplate.FindSnapshot(s.Context, s.providerSpec.Snapshot)
			if err != nil {
				// Maybe return an error there?
				klog.V(3).Infof("%v: failed to find snapshot %s, fallback to FullClone", s.machine.GetName(), s.providerSpec.Snapshot)
			}
		}

		if snapshotRef != nil {
			diskMoveType = linkCloneDiskMoveType
		}
	}

	var folderPath, datastorePath, resourcepoolPath string
	if s.providerSpec.Workspace != nil {
		folderPath = s.providerSpec.Workspace.Folder
		datastorePath = s.providerSpec.Workspace.Datastore
		resourcepoolPath = s.providerSpec.Workspace.ResourcePool
	}

	folder, err := s.GetSession().Finder.FolderOrDefault(s, folderPath)
	if err != nil {
		const multipleFoundMsg = "multiple folders found, specify one in config"
		const notFoundMsg = "folder not found, specify valid value"
		defaultError := fmt.Errorf("unable to get folder for %q: %w", folderPath, err)
		return "", handleVSphereError(multipleFoundMsg, notFoundMsg, defaultError, err)
	}

	datastore, err := s.GetSession().Finder.DatastoreOrDefault(s, datastorePath)
	if err != nil {
		const multipleFoundMsg = "multiple datastores found, specify one in config"
		const notFoundMsg = "datastore not found, specify valid value"
		defaultError := fmt.Errorf("unable to get datastore for %q: %w", datastorePath, err)
		return "", handleVSphereError(multipleFoundMsg, notFoundMsg, defaultError, err)
	}

	resourcepool, err := s.GetSession().Finder.ResourcePoolOrDefault(s, resourcepoolPath)
	if err != nil {
		const multipleFoundMsg = "multiple resource pools found, specify one in config"
		const notFoundMsg = "resource pool not found, specify valid value"
		defaultError := fmt.Errorf("unable to get resource pool for %q: %w", resourcepool, err)
		return "", handleVSphereError(multipleFoundMsg, notFoundMsg, defaultError, err)
	}

	numCPUs := s.providerSpec.NumCPUs

	numCoresPerSocket := s.providerSpec.NumCoresPerSocket
	if numCoresPerSocket == 0 {
		numCoresPerSocket = numCPUs
	}

	devices, err := vmTemplate.Device(s.Context)
	if err != nil {
		return "", fmt.Errorf("error getting devices %v", err)
	}

	// Create a new list of device specs for cloning the VM.
	deviceSpecs := []types.BaseVirtualDeviceConfigSpec{}

	// Only non-linked clones may expand the size of the template's disk.
	if snapshotRef == nil {
		diskSpec, err := getDiskSpec(s, devices)
		if err != nil {
			return "", fmt.Errorf("error getting disk spec for %q: %w", s.providerSpec.Snapshot, err)
		}
		deviceSpecs = append(deviceSpecs, diskSpec)
	}

	klog.V(3).Infof("Getting network devices")
	networkDevices, err := getNetworkDevices(s, resourcepool, devices)
	if err != nil {
		return "", fmt.Errorf("error getting network specs: %w", err)
	}

	deviceSpecs = append(deviceSpecs, networkDevices...)

	extraConfig := []types.BaseOptionValue{}

	extraConfig = append(extraConfig, IgnitionConfig(userData)...)
	extraConfig = append(extraConfig, &types.OptionValue{
		Key:   GuestInfoHostname,
		Value: s.machine.GetName(),
	})
	extraConfig = append(extraConfig, &types.OptionValue{
		Key:   StealClock,
		Value: "TRUE",
	})

	spec := types.VirtualMachineCloneSpec{
		Config: &types.VirtualMachineConfigSpec{
			Annotation: s.machine.GetName(),
			// Assign the clone's InstanceUUID the value of the Kubernetes Machine
			// object's UID. This allows lookup of the cloned VM prior to knowing
			// the VM's UUID.
			InstanceUuid:      string(s.machine.UID),
			Flags:             newVMFlagInfo(),
			ExtraConfig:       extraConfig,
			DeviceChange:      deviceSpecs,
			NumCPUs:           numCPUs,
			NumCoresPerSocket: numCoresPerSocket,
			MemoryMB:          s.providerSpec.MemoryMiB,
		},
		Location: types.VirtualMachineRelocateSpec{
			Datastore:    types.NewReference(datastore.Reference()),
			Folder:       types.NewReference(folder.Reference()),
			Pool:         types.NewReference(resourcepool.Reference()),
			DiskMoveType: diskMoveType,
		},
		PowerOn:  false, // Create powered off machine, for power it on later in "create" procedure
		Snapshot: snapshotRef,
	}

	task, err := vmTemplate.Clone(s, folder, s.machine.GetName(), spec)
	if err != nil {
		return "", fmt.Errorf("error triggering clone op for machine %v: %w", s, err)
	}
	taskVal := task.Reference().Value
	klog.V(3).Infof("%v: running task: %+v", s.machine.GetName(), taskVal)
	return taskVal, nil
}

func powerOn(s *machineScope) (string, error) {
	vmRef, err := findVM(s)
	if err != nil {
		if !isNotFound(err) {
			return "", err
		}
		return "", fmt.Errorf("vm not found during creation for powering on: %w", err)
	}

	datacenter := s.session.Datacenter
	if datacenter == nil { // if there is no dataceneter, fallback to old powerOn method via vm object
		vm := &virtualMachine{
			Context: s.Context,
			Obj:     object.NewVirtualMachine(s.session.Client.Client, vmRef),
			Ref:     vmRef,
		}

		return vm.powerOnVM()
	}

	overrideDRS := &types.OptionValue{
		Key:   string(types.ClusterPowerOnVmOptionOverrideAutomationLevel),
		Value: string(types.DrsBehaviorFullyAutomated),
	}
	task, err := datacenter.PowerOnVM(s.Context, []types.ManagedObjectReference{vmRef}, overrideDRS)
	if err != nil {
		return "", fmt.Errorf("error powering on %s vm: %w", s.machine.Name, err)
	}
	return task.Reference().Value, nil
}

func getDiskSpec(s *machineScope, devices object.VirtualDeviceList) (types.BaseVirtualDeviceConfigSpec, error) {
	disks := devices.SelectByType((*types.VirtualDisk)(nil))
	if len(disks) != 1 {
		return nil, fmt.Errorf("invalid disk count: %d", len(disks))
	}

	disk := disks[0].(*types.VirtualDisk)
	cloneCapacityKB := int64(s.providerSpec.DiskGiB) * 1024 * 1024
	if disk.CapacityInKB > cloneCapacityKB {
		return nil, machinecontroller.InvalidMachineConfiguration(
			"can't resize template disk down, initial capacity is larger: %dKiB > %dKiB",
			disk.CapacityInKB, cloneCapacityKB)
	}
	disk.CapacityInKB = cloneCapacityKB

	return &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationEdit,
		Device:    disk,
	}, nil
}

func getNetworkDevices(s *machineScope, resourcepool *object.ResourcePool, devices object.VirtualDeviceList) ([]types.BaseVirtualDeviceConfigSpec, error) {
	var networkDevices []types.BaseVirtualDeviceConfigSpec
	// Remove any existing NICs
	for _, dev := range devices.SelectByType((*types.VirtualEthernetCard)(nil)) {
		networkDevices = append(networkDevices, &types.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: types.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Add new NICs based on the machine config.
	for i := range s.providerSpec.Network.Devices {
		var ccrMo mo.ClusterComputeResource
		var backing types.BaseVirtualDeviceBackingInfo

		netSpec := &s.providerSpec.Network.Devices[i]
		klog.V(3).Infof("Adding device: %v", netSpec.NetworkName)

		clusterRef, err := resourcepool.Owner(s.Context)
		if err != nil {
			return nil, fmt.Errorf("unable to find cluster resource: %w", err)
		}

		clusterRes := object.NewClusterComputeResource(s.GetSession().Client.Client, clusterRef.Reference())
		err = clusterRes.Properties(s.Context, clusterRef.Reference(), []string{"network"}, &ccrMo)
		if err != nil {
			return nil, fmt.Errorf("unable to get list of networks in cluster: %w", err)
		}

		for _, netRef := range ccrMo.Network {
			// Use generic network object to get name
			genericNetwork := object.NewNetwork(s.GetSession().Client.Client, netRef)
			networkName, err := genericNetwork.ObjectName(s.Context)
			if err != nil {
				return nil, fmt.Errorf("unable to get network name: %w", err)
			}
			if netSpec.NetworkName == networkName {
				// Use more specific network reference to get Ethernet info
				ref := object.NewReference(s.GetSession().Client.Client, netRef)
				networkObject, ok := ref.(object.NetworkReference)
				if !ok {
					return nil, fmt.Errorf("unable to create new ethernet card backing info for network %q: network type failure: %s", netSpec.NetworkName, ref.Reference().Type)
				}

				backing, err = networkObject.EthernetCardBackingInfo(s.Context)
				if err != nil {
					return nil, fmt.Errorf("unable to create new ethernet card backing info for network %q: %w", netSpec.NetworkName, err)
				}
				break
			}
		}

		if backing == nil {
			return nil, machinecontroller.InvalidMachineConfiguration("unable to get network for %q", netSpec.NetworkName)
		}

		dev, err := object.EthernetCardTypes().CreateEthernetCard(ethCardType, backing)
		if err != nil {
			return nil, fmt.Errorf("unable to create new ethernet card %q for network %q: %w", ethCardType, netSpec.NetworkName, err)
		}

		// Get the actual NIC object. This is safe to assert without a check
		// because "object.EthernetCardTypes().CreateEthernetCard" returns a
		// "types.BaseVirtualEthernetCard" as a "types.BaseVirtualDevice".
		nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
		// Assign a temporary device key to ensure that a unique one will be
		// generated when the device is created.
		nic.Key = int32(i)

		networkDevices = append(networkDevices, &types.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
		})
		klog.V(3).Infof("Adding device: eth card type: %v, network spec: %+v, device info: %+v",
			ethCardType, netSpec, dev.GetVirtualDevice().Backing)
	}

	return networkDevices, nil
}

func newVMFlagInfo() *types.VirtualMachineFlagInfo {
	diskUUIDEnabled := true
	return &types.VirtualMachineFlagInfo{
		DiskUuidEnabled: &diskUUIDEnabled,
	}
}

func taskIsFinished(task *mo.Task) (bool, error) {
	if task == nil {
		return true, nil
	}

	// Otherwise the course of action is determined by the state of the task.
	klog.V(3).Infof("task: %v, state: %v, description-id: %v", task.Reference().Value, task.Info.State, task.Info.DescriptionId)
	switch task.Info.State {
	case types.TaskInfoStateQueued:
		return false, nil
	case types.TaskInfoStateRunning:
		return false, nil
	case types.TaskInfoStateSuccess:
		return true, nil
	case types.TaskInfoStateError:
		return true, errors.New(task.Info.Error.LocalizedMessage)
	default:
		return false, fmt.Errorf("task: %v, unknown state %v", task.Reference().Value, task.Info.State)
	}
}

func setProviderStatus(taskRef string, condition metav1.Condition, scope *machineScope, vm *virtualMachine) error {
	klog.Infof("%s: Updating provider status", scope.machine.Name)

	if vm != nil {
		id := vm.Obj.UUID(scope.Context)
		scope.providerStatus.InstanceID = &id

		// This can return an error if machine is being deleted
		powerState, err := vm.getPowerState()
		if err != nil {
			klog.V(3).Infof("%s: Failed to get power state during provider status update: %v", scope.machine.Name, err)
		} else {
			powerStateString := string(powerState)
			scope.providerStatus.InstanceState = &powerStateString
		}
	}

	if taskRef != "" {
		scope.providerStatus.TaskRef = taskRef
	}

	scope.providerStatus.Conditions = setConditions(condition, scope.providerStatus.Conditions)

	return nil
}

func handleVSphereError(multipleFoundMsg, notFoundMsg string, defaultError, vsphereError error) error {
	var multipleFoundError *find.MultipleFoundError
	if errors.As(vsphereError, &multipleFoundError) {
		return machinecontroller.InvalidMachineConfiguration(multipleFoundMsg)
	}

	var notFoundError *find.NotFoundError
	if errors.As(vsphereError, &notFoundError) {
		return machinecontroller.InvalidMachineConfiguration(notFoundMsg)
	}

	return defaultError
}

type virtualMachine struct {
	context.Context
	Ref types.ManagedObjectReference
	Obj *object.VirtualMachine
}

// getHostSystemAncestors looks up and returns vm's host system ancestors, such as "Cluster" and "Datacenter".
// Host system is using there because in vCenter cluster is an ancestor of the hypervisor host but not the vm.
func (vm *virtualMachine) getHostSystemAncestors() ([]mo.ManagedEntity, error) {
	client := vm.Obj.Client()
	pc := client.ServiceContent.PropertyCollector

	host, err := vm.Obj.HostSystem(vm.Context)
	if err != nil {
		return nil, err
	}

	return mo.Ancestors(vm.Context, client, pc, host.Reference())
}

// getRegionAndZone checks the virtual machine and each of its ancestors for the
// given region and zone labels and returns their values if found.
func (vm *virtualMachine) getRegionAndZone(tagsMgr *session.CachingTagsManager, regionLabel, zoneLabel string) (map[string]string, error) {
	result := make(map[string]string)

	objects, err := vm.getHostSystemAncestors()
	if err != nil {
		klog.Errorf("Failed to get ancestors for %s: %v", vm.Ref, err)
		return nil, err
	}

	for i := range objects {
		obj := objects[len(objects)-1-i] // Reverse order.
		klog.V(4).Infof("getRegionAndZone: Name: %s, Type: %s",
			obj.Self.Value, obj.Self.Type)

		tags, err := tagsMgr.ListAttachedTags(vm.Context, obj)
		if err != nil {
			klog.Warningf("Failed to list attached tags: %v", err)
			return nil, err
		}

		for _, value := range tags {
			tag, err := tagsMgr.GetTag(vm.Context, value)
			if err != nil {
				klog.Errorf("Failed to get tag: %v", err)
				return nil, err
			}

			category, err := tagsMgr.GetCategory(vm.Context, tag.CategoryID)
			if err != nil {
				klog.Errorf("Failed to get tag category: %v", err)
				return nil, err
			}

			switch {
			case regionLabel != "" && category.Name == regionLabel:
				result[regionKey] = tag.Name
				klog.V(2).Infof("%s has region tag (%s) with value %s",
					vm.Ref, category.Name, tag.Name)

			case zoneLabel != "" && category.Name == zoneLabel:
				result[zoneKey] = tag.Name
				klog.V(2).Infof("%s has zone tag (%s) with value %s",
					vm.Ref, category.Name, tag.Name)
			}

			// We've found both tags, return early.
			if result[regionKey] != "" && result[zoneKey] != "" {
				return result, nil
			}
		}
	}

	return result, nil
}

func (vm *virtualMachine) powerOnVM() (string, error) {
	task, err := vm.Obj.PowerOn(vm.Context)
	if err != nil {
		return "", err
	}
	return task.Reference().Value, nil
}

func (vm *virtualMachine) powerOffVM() (string, error) {
	task, err := vm.Obj.PowerOff(vm.Context)
	if err != nil {
		return "", err
	}
	return task.Reference().Value, nil
}

func (vm *virtualMachine) getPowerState() (types.VirtualMachinePowerState, error) {
	powerState, err := vm.Obj.PowerState(vm.Context)
	if err != nil {
		return "", err
	}

	switch powerState {
	case types.VirtualMachinePowerStatePoweredOn:
		return types.VirtualMachinePowerStatePoweredOn, nil
	case types.VirtualMachinePowerStatePoweredOff:
		return types.VirtualMachinePowerStatePoweredOff, nil
	case types.VirtualMachinePowerStateSuspended:
		return types.VirtualMachinePowerStateSuspended, nil
	default:
		return "", fmt.Errorf("unexpected power state %q for vm %v", powerState, vm)
	}
}

// reconcileTags ensures that the required tags are present on the virtual machine, eg the Cluster ID
// that is used by the installer on cluster deletion to ensure ther are no leaked resources.
func (vm *virtualMachine) reconcileTags(ctx context.Context, sessionInstance *session.Session, machine *machinev1.Machine) error {
	if err := sessionInstance.WithCachingTagsManager(vm.Context, func(c *session.CachingTagsManager) error {
		klog.Infof("%v: Reconciling attached tags", machine.GetName())

		clusterID := machine.Labels[machinev1.MachineClusterIDLabel]

		attached, err := vm.checkAttachedTag(ctx, clusterID, c)
		if err != nil {
			return err
		}

		if !attached {
			klog.Infof("%v: Attaching %s tag to vm", machine.GetName(), clusterID)
			// the tag should already be created by installer
			if err := c.AttachTag(ctx, clusterID, vm.Ref); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// checkAttachedTag returns true if tag is already attached to a vm or tag doesn't exist
func (vm *virtualMachine) checkAttachedTag(ctx context.Context, tagName string, m *session.CachingTagsManager) (bool, error) {
	// cluster ID tag doesn't exists in UPI, we should skip tag attachment if it's not found
	foundTag, err := vm.foundTag(ctx, tagName, m)
	if err != nil {
		return false, err
	}

	if !foundTag {
		return true, nil
	}

	tags, err := m.GetAttachedTags(ctx, vm.Ref)
	if err != nil {
		return false, err
	}

	for _, tag := range tags {
		if tag.Name == tagName {
			return true, nil
		}
	}

	return false, nil
}

// tagToCategoryName converts the tag name to the category name based upon the format set up by the installer.
// Note this is only valid in IPI clusters as typically a UPI cluster won't have the cluster ID tag, in which case the
// controller skips tag creation.
// Ref: https://github.com/openshift/installer/blob/f912534f12491721e3874e2bf64f7fa8d44aa7f5/data/data/vsphere/pre-bootstrap/main.tf#L57
// Ref: https://github.com/openshift/installer/blob/f912534f12491721e3874e2bf64f7fa8d44aa7f5/pkg/destroy/vsphere/vsphere.go#L231
func tagToCategoryName(tagName string) string {
	return fmt.Sprintf("openshift-%s", tagName)
}

func (vm *virtualMachine) foundTag(ctx context.Context, tagName string, m *session.CachingTagsManager) (bool, error) {
	tags, err := m.ListTagsForCategory(ctx, tagToCategoryName(tagName))
	if err != nil {
		if isNotFoundErr(err) {
			return false, nil
		}
		return false, err
	}

	for _, id := range tags {
		tag, err := m.GetTag(ctx, id)
		if err != nil {
			return false, err
		}

		if tag.Name == tagName {
			return true, nil
		}
	}

	return false, nil
}

type NetworkStatus struct {
	// Connected is a flag that indicates whether this network is currently
	// connected to the VM.
	Connected bool

	// IPAddrs is one or more IP addresses reported by vm-tools.
	IPAddrs []string

	// MACAddr is the MAC address of the network device.
	MACAddr string

	// NetworkName is the name of the network.
	NetworkName string
}

func (vm *virtualMachine) getNetworkStatusList(client *vim25.Client) ([]NetworkStatus, error) {
	var obj mo.VirtualMachine
	var pc = property.DefaultCollector(client)
	var props = []string{
		"config.hardware.device",
		"guest.net",
	}

	if err := pc.RetrieveOne(vm.Context, vm.Ref, props, &obj); err != nil {
		return nil, fmt.Errorf("unable to fetch props %v for vm %v: %w", props, vm.Ref, err)
	}
	klog.V(3).Infof("Getting network status: object reference: %v", obj.Reference().Value)
	if obj.Config == nil {
		return nil, errors.New("config.hardware.device is nil")
	}

	var networkStatusList []NetworkStatus
	for _, device := range obj.Config.Hardware.Device {
		if dev, ok := device.(types.BaseVirtualEthernetCard); ok {
			nic := dev.GetVirtualEthernetCard()
			klog.V(3).Infof("Getting network status: device: %v, macAddress: %v", nic.DeviceInfo.GetDescription().Summary, nic.MacAddress)
			netStatus := NetworkStatus{
				MACAddr: nic.MacAddress,
			}
			if obj.Guest != nil {
				klog.V(3).Infof("Getting network status: getting guest info")
				for _, i := range obj.Guest.Net {
					klog.V(3).Infof("Getting network status: getting guest info: network: %+v", i)
					if strings.EqualFold(nic.MacAddress, i.MacAddress) {
						//TODO: sanitizeIPAddrs
						netStatus.IPAddrs = i.IpAddress
						netStatus.NetworkName = i.Network
						netStatus.Connected = i.Connected
					}
				}
			}
			networkStatusList = append(networkStatusList, netStatus)
		}
	}

	return networkStatusList, nil
}

type attachedDisk struct {
	device   *types.VirtualDisk
	fileName string
	diskMode string
}

// Filters out disks that look like vm OS disk.
// VM os disks filename contains the machine name in it
// and has the format like "[DATASTORE] path-within-datastore/machine-name.vmdk".
// This is based on vSphere behavior, an OS disk file gets a name that equals the target VM name during the clone operation.
func filterOutVmOsDisk(attachedDisks []attachedDisk, machine *machinev1.Machine) []attachedDisk {
	var disks []attachedDisk

	for _, disk := range attachedDisks {
		if strings.HasSuffix(disk.fileName, fmt.Sprintf("/%s.vmdk", machine.GetName())) {
			continue
		}
		disks = append(disks, disk)
	}
	return disks
}

func (vm *virtualMachine) getAttachedDisks() ([]attachedDisk, error) {
	var attachedDiskList []attachedDisk
	devices, err := vm.Obj.Device(vm.Context)
	if err != nil {
		return nil, err
	}

	for _, disk := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		backingInfo := disk.GetVirtualDevice().Backing.(types.BaseVirtualDeviceFileBackingInfo).(*types.VirtualDiskFlatVer2BackingInfo)
		attachedDiskList = append(attachedDiskList, attachedDisk{
			device:   disk.(*types.VirtualDisk),
			fileName: backingInfo.FileName,
			diskMode: backingInfo.DiskMode,
		})
	}

	return attachedDiskList, nil
}

func (vm *virtualMachine) detachDisks(disks []attachedDisk) error {
	var errList []error

	for _, disk := range disks {
		klog.V(3).Infof("Detaching disk associated with file %v", disk.fileName)
		if err := vm.Obj.RemoveDevice(vm.Context, true, disk.device); err != nil {
			errList = append(errList, err)
			klog.Errorf("Failed to detach disk associated with file %v ", disk.fileName)
		} else {
			klog.V(3).Infof("Disk associated with file %v has been detached", disk.fileName)
		}
	}
	if len(errList) > 0 {
		return apimachineryutilerrors.NewAggregate(errList)
	}
	return nil
}

// IgnitionConfig returns a slice of option values that set the given data as
// the guest's ignition config.
func IgnitionConfig(data []byte) []types.BaseOptionValue {
	config := EncodeIgnitionConfig(data)

	if config == "" {
		return nil
	}

	return []types.BaseOptionValue{
		&types.OptionValue{
			Key:   GuestInfoIgnitionData,
			Value: config,
		},
		&types.OptionValue{
			Key:   GuestInfoIgnitionEncoding,
			Value: "base64",
		},
	}
}

// EncodeIgnitionConfig attempts to decode the given data until it looks to be
// plain-text, then returns a base64 encoded version of that plain-text.
func EncodeIgnitionConfig(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	for {
		decoded, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			break
		}

		data = decoded
	}

	return base64.StdEncoding.EncodeToString(data)
}
