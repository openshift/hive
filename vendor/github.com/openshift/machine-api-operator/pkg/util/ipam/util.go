package ipam

import (
	"context"
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	ipamv1beta1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1" //nolint:staticcheck

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureIPAddressClaim ensures an IPAddressClaim exists for the given claim name and pool
func EnsureIPAddressClaim(
	ctx context.Context,
	runtimeClient client.Client,
	claimName string,
	machine *machinev1.Machine,
	pool machinev1.AddressesFromPool) (*ipamv1beta1.IPAddressClaim, error) {
	claimKey := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      claimName,
	}
	ipAddressClaim := &ipamv1beta1.IPAddressClaim{}
	if err := runtimeClient.Get(ctx, claimKey, ipAddressClaim); err == nil {
		// If we found a claim, make sure it has owner field set.
		if len(ipAddressClaim.OwnerReferences) == 0 {
			klog.Infof("IPAddressClaim %s is missing owner field.  Updating to reference machine.", claimName)
			_ = AdoptOrphanClaim(ctx, runtimeClient, claimName, machine, ipAddressClaim)
		}
		return ipAddressClaim, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("error while getting IP address claim: %w", err)
	}

	klog.Infof("creating IPAddressClaim %s", claimName)
	gv := machinev1.SchemeGroupVersion
	machineRef := metav1.NewControllerRef(machine, gv.WithKind("Machine"))
	ipAddressClaim = &ipamv1beta1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				*machineRef,
			},
			Finalizers: []string{
				machinev1.IPClaimProtectionFinalizer,
			},
			Name:      claimName,
			Namespace: machine.Namespace,
		},
		Spec: ipamv1beta1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: &pool.Group,
				Kind:     pool.Resource,
				Name:     pool.Name,
			},
		},
	}
	if err := runtimeClient.Create(ctx, ipAddressClaim); err != nil {
		return nil, fmt.Errorf("unable to create IPAddressClaim: %w", err)
	}
	return ipAddressClaim, nil
}

// AdoptOrphanClaim updates the IPAddressClaim to belong to the specified machine.
func AdoptOrphanClaim(
	ctx context.Context,
	runtimeClient client.Client,
	claimName string,
	machine *machinev1.Machine,
	ipAddressClaim *ipamv1beta1.IPAddressClaim) error {
	gv := machinev1.SchemeGroupVersion
	machineRef := metav1.NewControllerRef(machine, gv.WithKind("Machine"))
	ipAddressClaim.OwnerReferences = []metav1.OwnerReference{
		*machineRef,
	}
	if err := runtimeClient.Update(ctx, ipAddressClaim); err != nil {
		klog.Warningf("Unable to adopt orphaned IPAddressClaim %v: %v", claimName, err)
		return err
	}
	return nil
}

// CountOutstandingIPAddressClaimsForMachine determines the number of outstanding IP address claims a machine is waiting
// to be fulfilled.
func CountOutstandingIPAddressClaimsForMachine(
	ctx context.Context,
	runtimeClient client.Client,
	ipAddressClaims []ipamv1beta1.IPAddressClaim) int {
	fulfilledClaimCount := 0

	for _, claim := range ipAddressClaims {
		if len(claim.Status.AddressRef.Name) > 0 {
			fulfilledClaimCount++
		}
	}

	return len(ipAddressClaims) - fulfilledClaimCount
}

// RetrieveBoundIPAddress retrieves the IPAddress which is bound to the named IPAddressClaim
func RetrieveBoundIPAddress(
	ctx context.Context,
	runtimeClient client.Client,
	machine *machinev1.Machine,
	claimName string) (*ipamv1beta1.IPAddress, error) {

	claimKey := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      claimName,
	}
	ipAddressClaim := &ipamv1beta1.IPAddressClaim{}
	if err := runtimeClient.Get(ctx, claimKey, ipAddressClaim); err != nil {
		return nil, fmt.Errorf("unable to get IPAddressClaim: %w", err)
	}

	if len(ipAddressClaim.Status.AddressRef.Name) == 0 {
		return nil, fmt.Errorf("no IPAddress is bound to claim %s", claimName)
	}

	ipAddress := &ipamv1beta1.IPAddress{}
	addressKey := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      ipAddressClaim.Status.AddressRef.Name,
	}
	if err := runtimeClient.Get(ctx, addressKey, ipAddress); err != nil {
		return nil, fmt.Errorf("unable to get IPAddress: %w", err)
	}
	return ipAddress, nil
}

func RemoveFinalizersForIPAddressClaims(
	ctx context.Context,
	runtimeClient client.Client,
	machine machinev1.Machine) error {
	ipAddressClaimList := &ipamv1beta1.IPAddressClaimList{}
	if err := runtimeClient.List(ctx, ipAddressClaimList, client.InNamespace(machine.Namespace)); err != nil {
		return fmt.Errorf("unable to list IPAddressClaims: %w", err)
	}

	for _, ipAddressClaim := range ipAddressClaimList.Items {
		if metav1.IsControlledBy(&ipAddressClaim, &machine) {
			finalizers := sets.NewString(ipAddressClaim.ObjectMeta.Finalizers...)
			if !finalizers.Has(machinev1.IPClaimProtectionFinalizer) {
				continue
			}
			finalizers.Delete(machinev1.IPClaimProtectionFinalizer)
			ipAddressClaim.ObjectMeta.Finalizers = finalizers.List()
			if err := runtimeClient.Update(ctx, &ipAddressClaim); err != nil {
				return fmt.Errorf("unable to update IPAddressClaim: %w", err)
			}
		}
	}
	return nil
}

// HasStaticIPConfiguration returns true if the specified machine has an address pool
func HasStaticIPConfiguration(providerSpec *machinev1.VSphereMachineProviderSpec) bool {
	for _, device := range providerSpec.Network.Devices {
		if len(device.Gateway) > 0 ||
			len(device.IPAddrs) > 0 ||
			len(device.Nameservers) > 0 ||
			len(device.AddressesFromPools) > 0 {
			return true
		}
	}
	return false
}

// GetIPAddressClaimName returns a consistently named claim name
func GetIPAddressClaimName(machine *machinev1.Machine, deviceIdx int, poolIdx int) string {
	return fmt.Sprintf("%s-claim-%d-%d", machine.Name, deviceIdx, poolIdx)
}

// HasOutstandingIPAddressClaims checks to see if a given machine has outstanding IP address claims
func HasOutstandingIPAddressClaims(
	ctx context.Context,
	runtimeClient client.Client,
	machine *machinev1.Machine,
	networkDevices []machinev1.NetworkDeviceSpec,
) (int, error) {
	var associatedClaims []ipamv1beta1.IPAddressClaim
	for deviceIdx, networkDevice := range networkDevices {
		for poolIdx, addressPool := range networkDevice.AddressesFromPools {
			claimName := GetIPAddressClaimName(machine, deviceIdx, poolIdx)
			claim, err := EnsureIPAddressClaim(
				ctx,
				runtimeClient,
				claimName,
				machine,
				addressPool)
			if err != nil {
				return -1, fmt.Errorf("error while ensuring IPAddressClaim exists: %w", err)
			}
			associatedClaims = append(associatedClaims, *claim)
		}
	}

	if len(associatedClaims) > 0 {
		outstandingClaims := CountOutstandingIPAddressClaimsForMachine(ctx, runtimeClient, associatedClaims)
		return outstandingClaims, nil
	}
	return 0, nil
}

// VerifyIPAddressOwners verifies that each IPAddress associated with the machine has the owner set.
func VerifyIPAddressOwners(
	ctx context.Context,
	runtimeClient client.Client,
	machine *machinev1.Machine,
	networkDevices []machinev1.NetworkDeviceSpec) error {
	for deviceIdx, networkDevice := range networkDevices {
		for poolIdx, addressPool := range networkDevice.AddressesFromPools {
			claimName := GetIPAddressClaimName(machine, deviceIdx, poolIdx)
			_, err := EnsureIPAddressClaim(
				ctx,
				runtimeClient,
				claimName,
				machine,
				addressPool)
			if err != nil {
				return fmt.Errorf("error while ensuring IPAddressClaim exists: %w", err)
			}
		}
	}
	return nil
}
