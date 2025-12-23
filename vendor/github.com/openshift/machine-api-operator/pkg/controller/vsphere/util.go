package vsphere

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	vsphere "k8s.io/cloud-provider-vsphere/pkg/common/config"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	globalInfrastuctureName  = "cluster"
	OpenshiftConfigNamespace = "openshift-config"
)

// GetInfrastructure retrieves the Infrastructure object from the provided API reader.
// It returns an error if the API reader is nil or if there's an issue fetching the Infrastructure.
func GetInfrastructure(c runtimeclient.Reader) (*configv1.Infrastructure, error) {
	if c == nil {
		return nil, errors.New("no API reader -- will not fetch infrastructure config")
	}

	infra := &configv1.Infrastructure{}
	infraName := runtimeclient.ObjectKey{Name: globalInfrastuctureName}

	if err := c.Get(context.Background(), infraName, infra); err != nil {
		return nil, err
	}

	return infra, nil
}

func getVSphereConfig(c runtimeclient.Reader, configNamespace string) (*vsphere.Config, error) {
	if c == nil {
		return nil, errors.New("no API reader -- will not fetch vSphere config")
	}

	infra, err := GetInfrastructure(c)
	if err != nil {
		return nil, err
	}

	if infra.Spec.CloudConfig.Name == "" {
		return nil, fmt.Errorf("cluster infrastructure CloudConfig has empty name")
	}

	if infra.Spec.CloudConfig.Key == "" {
		return nil, fmt.Errorf("cluster infrastructure CloudConfig has empty key")
	}

	cm := &corev1.ConfigMap{}
	cmName := runtimeclient.ObjectKey{
		Name:      infra.Spec.CloudConfig.Name,
		Namespace: configNamespace,
	}

	if err := c.Get(context.Background(), cmName, cm); err != nil {
		return nil, err
	}

	cloudConfig, found := cm.Data[infra.Spec.CloudConfig.Key]
	if !found {
		return nil, fmt.Errorf("cloud-config ConfigMap has no %q key",
			infra.Spec.CloudConfig.Key,
		)
	}

	return vsphere.ReadConfig([]byte(cloudConfig))
}

func setConditions(condition metav1.Condition, conditions []metav1.Condition) []metav1.Condition {
	now := metav1.Now()

	if existingCondition := findCondition(conditions, condition.Type); existingCondition == nil {
		condition.LastTransitionTime = now
		conditions = append(conditions, condition)
	} else {
		updateExistingCondition(&condition, existingCondition)
	}

	return conditions
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func updateExistingCondition(newCondition, existingCondition *metav1.Condition) {
	if !shouldUpdateCondition(newCondition, existingCondition) {
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = metav1.Now()
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}

func shouldUpdateCondition(newCondition, existingCondition *metav1.Condition) bool {
	return newCondition.Reason != existingCondition.Reason || newCondition.Message != existingCondition.Message
}

func conditionSuccess() metav1.Condition {
	return metav1.Condition{
		Type:    string(machinev1.MachineCreation),
		Status:  metav1.ConditionTrue,
		Reason:  machinev1.MachineCreationSucceededConditionReason,
		Message: "Machine successfully created",
	}
}

func conditionFailed() metav1.Condition {
	return metav1.Condition{
		Type:   string(machinev1.MachineCreation),
		Status: metav1.ConditionFalse,
		Reason: machinev1.MachineCreationSucceededConditionReason,
	}
}

func getVCenterPortFromConfig(config *vsphere.Config, vcenter string) string {
	if config != nil {
		for _, vc := range config.VirtualCenter {
			if vc.VCenterIP == vcenter {
				if len(vc.VCenterPort) > 0 {
					return vc.VCenterPort
				} else {
					return config.Global.VCenterPort
				}
			}
		}
	}
	return ""
}

func getVCenterInsecureFlagFromConfig(config *vsphere.Config, vcenter string) bool {
	if config != nil {
		for _, vc := range config.VirtualCenter {
			if vc.VCenterIP == vcenter {
				return vc.InsecureFlag
			}
		}

		// Either vCenter is not found or config is missing.
		return config.Global.InsecureFlag
	}

	return false
}

// RawExtensionFromProviderSpec marshals the machine provider spec.
func RawExtensionFromProviderSpec(spec *machinev1.VSphereMachineProviderSpec) (*runtime.RawExtension, error) {
	if spec == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(spec); err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// RawExtensionFromProviderStatus marshals the provider status
func RawExtensionFromProviderStatus(status *machinev1.VSphereMachineProviderStatus) (*runtime.RawExtension, error) {
	if status == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(status); err != nil {
		return nil, fmt.Errorf("error marshalling providerStatus: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// ProviderSpecFromRawExtension unmarshals the JSON-encoded spec
func ProviderSpecFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1.VSphereMachineProviderSpec, error) {
	if rawExtension == nil {
		return &machinev1.VSphereMachineProviderSpec{}, nil
	}

	spec := new(machinev1.VSphereMachineProviderSpec)
	if err := json.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}

	klog.V(5).Infof("Got provider spec from raw extension: %+v", spec)
	return spec, nil
}

// ProviderStatusFromRawExtension unmarshals a raw extension into a VSphereMachineProviderStatus type
func ProviderStatusFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1.VSphereMachineProviderStatus, error) {
	if rawExtension == nil {
		return &machinev1.VSphereMachineProviderStatus{}, nil
	}

	providerStatus := new(machinev1.VSphereMachineProviderStatus)
	if err := json.Unmarshal(rawExtension.Raw, providerStatus); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerStatus: %v", err)
	}

	klog.V(5).Infof("Got provider Status from raw extension: %+v", providerStatus)
	return providerStatus, nil
}

// isNotFoundErr checks if error message contains "Not Found" message.
// vSphere api client does not expose error type, so we can rely only on error message
func isNotFoundErr(err error) bool {
	return err != nil && strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}

// podPredicate is a predicate function for filtering PodList
type podPredicate func(corev1.Pod) bool

// isTerminating is a predicate for determine pods in 'Terminating' state
func isTerminating(p corev1.Pod) bool {
	return p.DeletionTimestamp != nil
}

// filterPods filters a podList and returns a PodList matching passed predicates
func filterPods(podList *corev1.PodList, predicates ...podPredicate) *corev1.PodList {
	filteredPods := &corev1.PodList{}
	for _, pod := range podList.Items {
		var match = true
		for _, p := range predicates {
			if !p(pod) {
				match = false
				break
			}
		}
		if match {
			filteredPods.Items = append(filteredPods.Items, pod)
		}
	}

	return filteredPods
}

// getPodList returns pod list on a given node matching pod predicates
func getPodList(ctx context.Context, apiReader runtimeclient.Reader, n *corev1.Node, filters []podPredicate) (*corev1.PodList, error) {
	fieldSelector, err := fields.ParseSelector("spec.nodeName=" + n.Name)
	if err != nil {
		return nil, err
	}

	allPods := &corev1.PodList{}
	if err := apiReader.List(ctx, allPods, &runtimeclient.ListOptions{
		FieldSelector: fieldSelector,
	}); err != nil {
		return nil, err
	}

	return filterPods(allPods, filters...), nil
}
