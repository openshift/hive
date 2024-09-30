package vsphere

// This is a thin layer to implement the machine actuator interface with cloud provider details.
// The lifetime of scope and reconciler is a machine actuator operation.
import (
	"context"
	"fmt"
	"time"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	scopeFailFmt        = "%s: failed to create scope for machine: %v"
	reconcilerFailFmt   = "%s: reconciler failed to %s machine: %w"
	createEventAction   = "Create"
	updateEventAction   = "Update"
	deleteEventAction   = "Delete"
	noEventAction       = ""
	requeueAfterSeconds = 20
)

// Actuator is responsible for performing machine reconciliation.
type Actuator struct {
	client                     runtimeclient.Client
	apiReader                  runtimeclient.Reader
	eventRecorder              record.EventRecorder
	TaskIDCache                map[string]string
	StaticIPFeatureGateEnabled bool
	openshiftConfigNamespace   string
}

// ActuatorParams holds parameter information for Actuator.
type ActuatorParams struct {
	Client                     runtimeclient.Client
	APIReader                  runtimeclient.Reader
	EventRecorder              record.EventRecorder
	TaskIDCache                map[string]string
	StaticIPFeatureGateEnabled bool
	OpenshiftConfigNamespace   string
}

// NewActuator returns an actuator.
func NewActuator(params ActuatorParams) *Actuator {
	return &Actuator{
		client:                     params.Client,
		apiReader:                  params.APIReader,
		eventRecorder:              params.EventRecorder,
		TaskIDCache:                params.TaskIDCache,
		StaticIPFeatureGateEnabled: params.StaticIPFeatureGateEnabled,
		openshiftConfigNamespace:   params.OpenshiftConfigNamespace,
	}
}

// Set corresponding event based on error. It also returns the original error
// for convenience, so callers can do "return handleMachineError(...)".
func (a *Actuator) handleMachineError(machine *machinev1.Machine, err error, eventAction string) error {
	klog.Errorf("%q error: %v", machine.GetName(), err)
	if eventAction != noEventAction {
		a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err)
	}
	return err
}

// Create creates a machine and is invoked by the machine controller.
func (a *Actuator) Create(ctx context.Context, machine *machinev1.Machine) error {
	klog.Infof("%s: actuator creating machine", machine.GetName())

	scope, err := newMachineScope(machineScopeParams{
		Context:                    ctx,
		client:                     a.client,
		machine:                    machine,
		apiReader:                  a.apiReader,
		StaticIPFeatureGateEnabled: a.StaticIPFeatureGateEnabled,
		openshiftConfigNameSpace:   a.openshiftConfigNamespace,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, createEventAction)
	}

	// Ensure we're not reconciling a stale machine by checking our task-id.
	// This is a workaround for a cache race condition.
	if val, ok := a.TaskIDCache[machine.Name]; ok {
		if val != scope.providerStatus.TaskRef {
			klog.Errorf("%s: machine object missing expected provider task ID, requeue", machine.GetName())
			return &machinecontroller.RequeueAfterError{RequeueAfter: requeueAfterSeconds * time.Second}
		}
	}

	var retErr error
	err = newReconciler(scope).create()
	// save the taskRef in our cache in case of any error with patch.
	if scope.providerStatus.TaskRef != "" {
		a.TaskIDCache[machine.Name] = scope.providerStatus.TaskRef
	}
	if err != nil {
		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), createEventAction, err)
		retErr = a.handleMachineError(machine, fmtErr, createEventAction)
	} else {
		a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, createEventAction, "Created Machine %v", machine.GetName())
	}

	if err := scope.PatchMachine(); err != nil {
		return err
	}

	return retErr
}

func (a *Actuator) Exists(ctx context.Context, machine *machinev1.Machine) (bool, error) {
	klog.Infof("%s: actuator checking if machine exists", machine.GetName())
	scope, err := newMachineScope(machineScopeParams{
		Context:                    ctx,
		client:                     a.client,
		machine:                    machine,
		apiReader:                  a.apiReader,
		StaticIPFeatureGateEnabled: a.StaticIPFeatureGateEnabled,
		openshiftConfigNameSpace:   a.openshiftConfigNamespace,
	})
	if err != nil {
		return false, fmt.Errorf(scopeFailFmt, machine.GetName(), err)
	}
	return newReconciler(scope).exists()
}

func (a *Actuator) Update(ctx context.Context, machine *machinev1.Machine) error {
	klog.Infof("%s: actuator updating machine", machine.GetName())
	// Cleanup TaskIDCache so we don't continually grow
	delete(a.TaskIDCache, machine.Name)

	scope, err := newMachineScope(machineScopeParams{
		Context:                    ctx,
		client:                     a.client,
		machine:                    machine,
		apiReader:                  a.apiReader,
		StaticIPFeatureGateEnabled: a.StaticIPFeatureGateEnabled,
		openshiftConfigNameSpace:   a.openshiftConfigNamespace,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, updateEventAction)
	}
	if err := newReconciler(scope).update(); err != nil {
		// Update machine and machine status in case it was modified
		if err := scope.PatchMachine(); err != nil {
			return err
		}
		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), updateEventAction, err)
		return a.handleMachineError(machine, fmtErr, updateEventAction)
	}
	previousResourceVersion := scope.machine.ResourceVersion

	if err := scope.PatchMachine(); err != nil {
		return err
	}

	currentResourceVersion := scope.machine.ResourceVersion

	// Create event only if machine object was modified
	if previousResourceVersion != currentResourceVersion {
		a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, updateEventAction, "Updated Machine %v", machine.GetName())
	}

	return nil
}

func (a *Actuator) Delete(ctx context.Context, machine *machinev1.Machine) error {
	klog.Infof("%s: actuator deleting machine", machine.GetName())
	// Cleanup TaskIDCache so we don't continually grow
	// Cleanup here as well in case Update() was never successfully called.
	delete(a.TaskIDCache, machine.Name)

	scope, err := newMachineScope(machineScopeParams{
		Context:                    ctx,
		client:                     a.client,
		machine:                    machine,
		apiReader:                  a.apiReader,
		StaticIPFeatureGateEnabled: a.StaticIPFeatureGateEnabled,
		openshiftConfigNameSpace:   a.openshiftConfigNamespace,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, deleteEventAction)
	}
	if err := newReconciler(scope).delete(); err != nil {
		if err := scope.PatchMachine(); err != nil {
			return err
		}
		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), deleteEventAction, err)
		return a.handleMachineError(machine, fmtErr, deleteEventAction)
	}
	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, deleteEventAction, "Deleted machine %v", machine.GetName())
	return scope.PatchMachine()
}
