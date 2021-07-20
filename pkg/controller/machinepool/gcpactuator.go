package machinepool

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	gcpprovider "github.com/openshift/cluster-api-provider-gcp/pkg/apis"
	gcpproviderv1beta1 "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"

	installgcp "github.com/openshift/installer/pkg/asset/machines/gcp"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesgcp "github.com/openshift/installer/pkg/types/gcp"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	// Omit m, the installer used this for the master machines. w is also removed as this is implicitly used
	// by the installer for the original worker pool.
	validLeaseChars = "abcdefghijklnopqrstuvxyz0123456789"

	defaultGCPDiskType   = "pd-ssd"
	defaultGCPDiskSizeGB = 128
)

var (
	versionsSupportingFullNames = semver.MustParseRange(">=4.4.7")
)

// GCPActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type GCPActuator struct {
	client    client.Client
	gcpClient gcpclient.Client
	logger    log.FieldLogger
	scheme    *runtime.Scheme
	projectID string
	imageID   string
	network   string
	subnet    string
	// expectations is a reference to the reconciler's TTLCache of machinepoolnamelease creates each machinepool
	// expects to see.
	expectations   controllerutils.ExpectationsInterface
	leasesRequired bool
}

var _ Actuator = &GCPActuator{}

func addGCPProviderToScheme(scheme *runtime.Scheme) error {
	return gcpprovider.AddToScheme(scheme)
}

// NewGCPActuator is the constructor for building a GCPActuator
func NewGCPActuator(
	client client.Client,
	gcpCreds *corev1.Secret,
	clusterVersion string,
	masterMachine *machineapi.Machine,
	remoteMachineSets []machineapi.MachineSet,
	scheme *runtime.Scheme,
	expectations controllerutils.ExpectationsInterface,
	logger log.FieldLogger,
) (*GCPActuator, error) {
	gcpClient, err := gcpclient.NewClientFromSecret(gcpCreds)
	if err != nil {
		logger.WithError(err).Warn("failed to create GCP client with creds in clusterDeployment's secret")
		return nil, err
	}

	projectID, err := gcpclient.ProjectIDFromSecret(gcpCreds)
	if err != nil {
		logger.WithError(err).Error("error getting project ID from GCP credentials secret")
		return nil, err
	}

	imageID, err := getGCPImageID(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting image ID from master machine")
		return nil, err
	}

	network, subnet, err := getNetwork(remoteMachineSets, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting network information from remote machines")
		return nil, err
	}

	actuator := &GCPActuator{
		gcpClient:      gcpClient,
		client:         client,
		logger:         logger,
		scheme:         scheme,
		expectations:   expectations,
		projectID:      projectID,
		imageID:        imageID,
		network:        network,
		subnet:         subnet,
		leasesRequired: requireLeases(clusterVersion, remoteMachineSets, logger),
	}
	return actuator, nil
}

// GenerateCAPIMachineSets takes a clusterDeployment and returns a list of upstream CAPI MachineSets
func (a *GCPActuator) GenerateCAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*capiv1.MachineSet, bool, error) {
	machinesets := []*capiv1.MachineSet{}
	return machinesets, true, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *GCPActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.GCP == nil {
		return nil, false, errors.New("ClusterDeployment is not for GCP")
	}
	if pool.Spec.Platform.GCP == nil {
		return nil, false, errors.New("MachinePool is not for GCP")
	}

	leases := &hivev1.MachinePoolNameLeaseList{}
	if err := a.client.List(
		context.TODO(),
		leases,
		client.InNamespace(pool.Namespace),
		client.MatchingLabels(map[string]string{
			constants.ClusterDeploymentNameLabel: cd.Name,
		}),
	); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error fetching machinepoolleases")
		return nil, false, err
	}

	poolName := pool.Spec.Name

	// If leases are required by the cluster version or existing "w" worker machinesets in the cluster or are already
	// being used as indicated by the existence of MachinePoolLeases, then use leases for determining the machine pool
	// name.
	useLeases := true
	switch {
	case a.leasesRequired:
		logger.Debug("using leases since they are required by the cluster")
	case len(leases.Items) > 0:
		logger.Debug("using leases since there are existing MachinePoolNameLeases")
	default:
		logger.Debug("not using leases")
		useLeases = false
	}
	if useLeases {
		leaseChar, proceed, err := a.obtainLease(pool, cd, leases)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error obtaining pool name lease")
			return nil, false, err
		}
		if !proceed {
			return nil, false, nil
		}
		poolName = leaseChar
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			GCP: &installertypesgcp.Platform{
				Region:        cd.Spec.Platform.GCP.Region,
				ProjectID:     a.projectID,
				ComputeSubnet: a.subnet,
				Network:       a.network,
			},
		},
	}

	computePool := baseMachinePool(pool)
	computePool.Name = poolName
	computePool.Platform.GCP = &installertypesgcp.MachinePool{
		Zones:        pool.Spec.Platform.GCP.Zones,
		InstanceType: pool.Spec.Platform.GCP.InstanceType,
		// May be overridden below:
		OSDisk: installertypesgcp.OSDisk{
			DiskType:   defaultGCPDiskType,
			DiskSizeGB: defaultGCPDiskSizeGB,
		},
	}

	poolGCP := pool.Spec.Platform.GCP
	if pool.Spec.Platform.GCP.OSDisk.DiskType != "" {
		computePool.Platform.GCP.OSDisk.DiskType = poolGCP.OSDisk.DiskType
	}
	if pool.Spec.Platform.GCP.OSDisk.DiskSizeGB != 0 {
		computePool.Platform.GCP.OSDisk.DiskSizeGB = poolGCP.OSDisk.DiskSizeGB
	}

	poolEncRef := poolGCP.OSDisk.EncryptionKey
	if poolEncRef != nil {
		computePool.Platform.GCP.OSDisk.EncryptionKey = &installertypesgcp.EncryptionKeyReference{
			KMSKeyServiceAccount: poolEncRef.KMSKeyServiceAccount,
		}
		if poolEncRef.KMSKey != nil {
			computePool.Platform.GCP.OSDisk.EncryptionKey.KMSKey = &installertypesgcp.KMSKeyReference{
				Name:      poolEncRef.KMSKey.Name,
				KeyRing:   poolEncRef.KMSKey.KeyRing,
				ProjectID: poolEncRef.KMSKey.ProjectID,
				Location:  poolEncRef.KMSKey.Location,
			}
		}
	}

	if len(computePool.Platform.GCP.Zones) == 0 {
		zones, err := a.getZones(cd.Spec.Platform.GCP.Region)
		if err != nil {
			return nil, false, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, false, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.GCP.Region)
		}
		computePool.Platform.GCP.Zones = zones
	}

	// Assuming all machine pools are workers at this time.
	installerMachineSets, err := installgcp.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		a.imageID,
		workerRole,
		workerUserDataName,
	)
	return installerMachineSets, err == nil, errors.Wrap(err, "failed to generate machinesets")
}

func (a *GCPActuator) getZones(region string) ([]string, error) {
	zones := []string{}

	// Filter to regions matching '.*<region>.*' (where the zone is actually UP)
	zoneFilter := fmt.Sprintf("(region eq '.*%s.*') (status eq UP)", region)

	pageToken := ""

	for {
		zoneList, err := a.gcpClient.ListComputeZones(gcpclient.ListComputeZonesOptions{
			Filter:    zoneFilter,
			PageToken: pageToken,
		})
		if err != nil {
			return zones, err
		}

		for _, zone := range zoneList.Items {
			zones = append(zones, zone.Name)
		}

		if zoneList.NextPageToken == "" {
			break
		}
		pageToken = zoneList.NextPageToken
	}

	return zones, nil
}

// obtainLease uses the Hive MachinePoolNameLease resource to obtain a unique, single character
// for use in the name of the machine pool. We are severely restricted on name lengths on GCP
// and effectively have one character of flexibility with the naming convention originating in
// the installer.
func (a *GCPActuator) obtainLease(pool *hivev1.MachinePool, cd *hivev1.ClusterDeployment, leases *hivev1.MachinePoolNameLeaseList) (leaseChar string, proceed bool, leaseErr error) {
	for _, l := range leases.Items {
		if l.Labels[constants.MachinePoolNameLabel] == pool.Name {
			a.logger.Debugf("machine pool already has lease: %s", l.Name)
			// Ensure the lease name is in the format we expect, we know everything up to
			// the last character.
			leaseChar := l.Name[len(l.Name)-1:]
			expectedLeaseName := fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, leaseChar)
			if expectedLeaseName != l.Name {
				return "", false, fmt.Errorf("lease %s did not match expected lease name format (%s[CHAR])", l.Name, expectedLeaseName)
			}
			return leaseChar, true, nil
		}
	}

	a.logger.Debugf("machine pool does not have a lease yet")

	var leaseRune rune
	// If the pool.Spec.Name == "worker", we want to preserve this MachinePool's "w" character that
	// the installer would have selected so we do not cycle all worker nodes.
	// Despite the separation of pool.Name and pool.Spec.Name, we do know that only one pool will
	// have pool.Spec.Name worker as we validate that the pool must be named
	// [clusterdeploymentname]-[pool.spec.name]
	if pool.Spec.Name == "worker" {
		leaseRune = 'w'
		a.logger.Debug("selecting lease char 'w' for original worker pool")
	} else {
		// Pool does not have a lease yet, lookup all currently available lease chars
		availLeaseChars, err := a.findAvailableLeaseChars(cd, leases)
		if err != nil {
			return "", false, err
		}
		if len(availLeaseChars) == 0 {
			a.logger.Warn("no GCP MachinePoolNameLease characters available, setting condition")
			conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
				pool.Status.Conditions,
				hivev1.NoMachinePoolNameLeasesAvailable,
				corev1.ConditionTrue,
				"OutOfMachinePoolNames",
				"All machine pool names are in use",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
			if changed {
				pool.Status.Conditions = conds
				if err := a.client.Status().Update(context.Background(), pool); err != nil {
					return "", false, err
				}
			}
			// Nothing else we can do, wait for requeue when a lease frees up
			return "", false, nil
		}
		// Ensure the above condition is not set if it shouldn't be.
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.NoMachinePoolNameLeasesAvailable,
			corev1.ConditionFalse,
			"MachinePoolNamesAvailable",
			"Machine pool names available",
			controllerutils.UpdateConditionNever,
		)
		if changed {
			pool.Status.Conditions = conds
			err := a.client.Status().Update(context.Background(), pool)
			if err != nil {
				return "", false, err
			}
		}

		// Choose a random entry in the available chars to limit collisions while processing
		// multiple machine pools at the same time. In this case the subsequent attempts to create
		// the lease will fail due to name collisions and re-reconcile.
		a.logger.Debug("selecting random lease char from available")
		leaseRune = availLeaseChars[rand.Intn(len(availLeaseChars))]
	}
	a.logger.Debugf("selected lease char: %s", string(leaseRune))

	leaseName := fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, string(leaseRune))
	// Attempt to claim the lease:
	l := &hivev1.MachinePoolNameLease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: pool.Namespace,
			Labels: map[string]string{
				constants.MachinePoolNameLabel:       pool.Name,
				constants.ClusterDeploymentNameLabel: cd.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "MachinePool",
					Name:       pool.Name,
					UID:        pool.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
	}
	a.logger.Debug("adding expectation for lease creation for this pool")
	expectKey := types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}.String()
	a.expectations.ExpectCreations(expectKey, 1)
	if err := a.client.Create(context.TODO(), l); err != nil {
		a.expectations.DeleteExpectations(expectKey)
		return "", false, err
	}
	a.logger.WithField("lease", leaseName).Infof("created lease, waiting until creation is observed")

	return string(leaseRune), false, nil
}

func stringToRuneSet(s string) map[rune]bool {
	chars := make(map[rune]bool)
	for _, char := range s {
		chars[char] = true
	}
	return chars
}

func (a *GCPActuator) findAvailableLeaseChars(cd *hivev1.ClusterDeployment, leases *hivev1.MachinePoolNameLeaseList) ([]rune, error) {
	availChars := stringToRuneSet(validLeaseChars)

	for _, lease := range leases.Items {
		if lease.Labels[constants.ClusterDeploymentNameLabel] != cd.Name {
			// doesn't match this cluster, shouldn't be possible due to filtering in caller, but
			// just in case
			continue
		}

		// Lease name is [infraid]-[x], we need to parse out the 'x'.
		char := lease.Name[len(lease.Name)-1]
		delete(availChars, rune(char))
	}

	keys := make([]rune, len(availChars))
	i := 0
	for k := range availChars {
		keys[i] = k
		i++
	}

	return keys, nil
}

func requireLeases(clusterVersion string, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) bool {
	logger = logger.WithField("clusterVersion", clusterVersion)
	if v, err := semver.ParseTolerant(clusterVersion); err == nil {
		if !versionsSupportingFullNames(v) {
			logger.Debug("leases are required since cluster does not support full machine names")
			return true
		}
	}
	poolNames := make(map[string]bool)
	for _, ms := range remoteMachineSets {
		nameParts := strings.Split(ms.Name, "-")
		if len(nameParts) < 3 {
			continue
		}
		poolName := nameParts[len(nameParts)-2]
		poolNames[poolName] = true
	}
	// If there are machinesets with a pool name of "w" and no machinesets with a pool name of "worker", then assume
	// that the "w" pool is the worker pool created by the installer. If the installer-created "w" worker pool still
	// exists, then we must continue to use leases.
	// This will cause problems if a machineset is created on the cluster with a "w" pool name that is not the
	// installer-created worker pool when there are Hive-managed pools that are not using leases. Hive will block
	// through validation MachinePools with a pool name of "w", but the user could still create such machinesets on
	// the cluster manually.
	if poolNames["w"] && !poolNames["worker"] {
		logger.Debug("leases are required since there is a \"w\" machine pool in the cluster that is likely the installer-created worker pool")
		return true
	}
	logger.Debug("leases are not required")
	return false
}

// Get the image ID from an existing master machine.
func getGCPImageID(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeGCPMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
	if err != nil {
		logger.WithError(err).Warn("cannot decode GCPMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode GCPMachineProviderSpec from master machine")
	}
	if len(providerSpec.Disks) == 0 {
		logger.Warn("master machine does not have any disks")
		return "", errors.New("master machine does not have any disks")
	}
	imageID := providerSpec.Disks[0].Image
	logger.WithField("image", imageID).Debug("resolved image to use for new machinesets")
	return imageID, nil
}

// getNetwork retrieves the network information (Network name and subnet)
// from existing machines on the remote cluster.
func getNetwork(remoteMachineSets []machineapi.MachineSet,
	scheme *runtime.Scheme, logger log.FieldLogger) (string, string, error) {
	if len(remoteMachineSets) == 0 {
		return "", "", nil
	}

	remoteMachineProviderSpec, err := decodeGCPMachineProviderSpec(
		remoteMachineSets[0].Spec.Template.Spec.ProviderSpec.Value,
		scheme,
	)
	if err != nil {
		logger.WithError(err).Warn("cannot decode GCPMachineProviderSpec from remote machinesets")
		return "", "", errors.Wrap(err, "cannot decode GCPMachineProviderSpec from remote machinesets")
	}

	if len(remoteMachineProviderSpec.NetworkInterfaces) == 0 {
		logger.Warn("remote machine do not have any network interfaces")
		return "", "", errors.Wrap(err, "remote machine do not have any network interfaces")
	}

	network := remoteMachineProviderSpec.NetworkInterfaces[0].Network
	subnet := remoteMachineProviderSpec.NetworkInterfaces[0].Subnetwork
	return network, subnet, nil
}

func decodeGCPMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*gcpproviderv1beta1.GCPMachineProviderSpec, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(gcpproviderv1beta1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode GCP ProviderSpec: %v", err)
	}
	spec, ok := obj.(*gcpproviderv1beta1.GCPMachineProviderSpec)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}
