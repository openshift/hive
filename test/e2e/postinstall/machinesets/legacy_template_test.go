package machinesets

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	sigsclient "sigs.k8s.io/controller-runtime/pkg/client"

	machinev1 "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

const vsphereLegacyMachinePoolName = "legacy-backfill"

// TestVSphereLegacyTemplateMachinePool proves that Hive can add capacity to a
// vSphere cluster whose install-time RHCOS template uses the legacy
// <infraID>-rhcos-<region>-<zone> name. The ClusterDeployment intentionally
// leaves Topology.Template empty so Hive must recover the template from the
// existing worker MachineSet.
func TestVSphereLegacyTemplateMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	if cd.Spec.Platform.VSphere == nil {
		t.Log("legacy template verification only applies to vSphere")
		return
	}

	require.NotNil(t, cd.Spec.Platform.VSphere.Infrastructure, "vSphere infrastructure is missing")
	require.NotNil(t, cd.Spec.ClusterMetadata, "cluster metadata is missing")
	failureDomains := cd.Spec.Platform.VSphere.Infrastructure.FailureDomains
	require.Len(t, failureDomains, 1, "focused fixture requires exactly one vSphere failure domain")

	expectedLegacyNames := make(map[string]struct{}, len(failureDomains))
	newFormatNames := make(map[string]struct{}, len(failureDomains))
	for _, failureDomain := range failureDomains {
		require.Empty(t, failureDomain.Topology.Template, "fixture must leave Topology.Template empty for failure domain %s", failureDomain.Name)
		require.NotEmpty(t, failureDomain.Region, "fixture failure domain %s has no region", failureDomain.Name)
		require.NotEmpty(t, failureDomain.Zone, "fixture failure domain %s has no zone", failureDomain.Name)
		expectedLegacyNames[fmt.Sprintf("%s-rhcos-%s-%s", cd.Spec.ClusterMetadata.InfraID, failureDomain.Region, failureDomain.Zone)] = struct{}{}
		newFormatNames[fmt.Sprintf("%s-rhcos-%s", cd.Spec.ClusterMetadata.InfraID, failureDomain.Name)] = struct{}{}
	}

	cfg := common.MustGetClusterDeploymentClientConfig()
	remoteClient := common.MustGetClientFromConfig(cfg)
	workerPrefix, err := machineNamePrefix(cd, workerMachinePoolName)
	require.NoError(t, err)

	workerMachineSets := listMachineSetsWithPrefix(t, remoteClient, workerPrefix)
	require.Len(t, workerMachineSets, len(failureDomains), "fixture must have one worker MachineSet per failure domain")

	legacyTemplates := make(map[string]struct{}, len(workerMachineSets))
	for i := range workerMachineSets {
		provider := mustVSphereProviderSpec(t, &workerMachineSets[i])
		require.NotEmpty(t, provider.Template, "worker MachineSet %s has no template", workerMachineSets[i].Name)
		templateName := path.Base(provider.Template)
		require.Contains(t, expectedLegacyNames, templateName, "worker MachineSet %s does not use a legacy region/zone template", workerMachineSets[i].Name)
		require.NotContains(t, newFormatNames, templateName, "worker MachineSet %s unexpectedly uses the failure-domain template name", workerMachineSets[i].Name)
		legacyTemplates[provider.Template] = struct{}{}
		t.Logf("worker MachineSet %s uses legacy template %s", workerMachineSets[i].Name, provider.Template)
	}

	hiveClient := common.MustGetClient()
	require.Nil(t, common.GetMachinePool(hiveClient, cd, vsphereLegacyMachinePoolName), "temporary MachinePool already exists")
	workerPool := common.GetMachinePool(hiveClient, cd, workerMachinePoolName)
	require.NotNil(t, workerPool, "worker MachinePool does not exist")
	require.NotNil(t, workerPool.Spec.Platform.VSphere, "worker MachinePool has no vSphere platform")

	pool := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cd.Namespace,
			Name:      fmt.Sprintf("%s-%s", cd.Name, vsphereLegacyMachinePoolName),
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{Name: cd.Name},
			Name:                 vsphereLegacyMachinePoolName,
			Replicas:             ptr.To(int64(1)),
			Platform:             *workerPool.Spec.Platform.DeepCopy(),
		},
	}

	require.NoError(t, hiveClient.Create(context.TODO(), pool), "cannot create legacy-template MachinePool")
	machinePrefix, err := machineNamePrefix(cd, vsphereLegacyMachinePoolName)
	require.NoError(t, err)
	logger := log.WithField("test", "TestVSphereLegacyTemplateMachinePool")

	t.Cleanup(func() {
		current := common.GetMachinePool(hiveClient, cd, vsphereLegacyMachinePoolName)
		if current != nil {
			if err := hiveClient.Delete(context.TODO(), current); err != nil && !apierrors.IsNotFound(err) {
				t.Errorf("cannot delete temporary MachinePool: %v", err)
				return
			}
		}
		if err := common.WaitForMachineSets(cfg, func(machineSets []*machinev1.MachineSet) bool {
			for _, machineSet := range machineSets {
				if strings.HasPrefix(machineSet.Name, machinePrefix) {
					return false
				}
			}
			return true
		}, 10*time.Minute); err != nil {
			t.Errorf("temporary MachineSets were not removed: %v", err)
		}
	})

	err = common.WaitForMachineSets(cfg, func(machineSets []*machinev1.MachineSet) bool {
		count := 0
		for _, machineSet := range machineSets {
			if strings.HasPrefix(machineSet.Name, machinePrefix) {
				count++
			}
		}
		return count == len(failureDomains)
	}, 5*time.Minute)
	require.NoError(t, err, "timed out waiting for the legacy-template MachineSets")

	generatedMachineSets := listMachineSetsWithPrefix(t, remoteClient, machinePrefix)
	require.Len(t, generatedMachineSets, len(failureDomains), "unexpected number of legacy-template MachineSets")
	for i := range generatedMachineSets {
		provider := mustVSphereProviderSpec(t, &generatedMachineSets[i])
		require.Contains(t, legacyTemplates, provider.Template, "MachineSet %s did not preserve an install-time worker template", generatedMachineSets[i].Name)
		t.Logf("generated MachineSet %s preserved template %s", generatedMachineSets[i].Name, provider.Template)
	}

	require.NoError(t, waitForMachines(logger, cfg, cd, machinePrefix, 1), "timed out waiting for the legacy-template machine")
	require.NoError(t, waitForNodes(logger, cfg, cd, machinePrefix, 1, nodeIsReady), "timed out waiting for the legacy-template node to become Ready")

	err = common.WaitForMachineSets(cfg, func(machineSets []*machinev1.MachineSet) bool {
		var readyReplicas int32
		var availableReplicas int32
		for _, machineSet := range machineSets {
			if strings.HasPrefix(machineSet.Name, machinePrefix) {
				readyReplicas += machineSet.Status.ReadyReplicas
				availableReplicas += machineSet.Status.AvailableReplicas
			}
		}
		return readyReplicas == 1 && availableReplicas == 1
	}, 5*time.Minute)
	require.NoError(t, err, "legacy-template MachineSet did not reach READY: 1 and AVAILABLE: 1")
	t.Log("legacy-template MachineSet reached READY: 1 and AVAILABLE: 1 with a Ready node")

	captureManifests(t, remoteClient, fmt.Sprintf("vsphere_legacy_%s_%s", cd.Namespace, cd.Name))
}

func listMachineSetsWithPrefix(t *testing.T, c sigsclient.Client, prefix string) []machinev1.MachineSet {
	t.Helper()
	machineSets := &machinev1.MachineSetList{}
	require.NoError(t, c.List(context.TODO(), machineSets, sigsclient.InNamespace("openshift-machine-api")))
	result := make([]machinev1.MachineSet, 0, len(machineSets.Items))
	for i := range machineSets.Items {
		if strings.HasPrefix(machineSets.Items[i].Name, prefix) {
			result = append(result, *machineSets.Items[i].DeepCopy())
		}
	}
	return result
}

func mustVSphereProviderSpec(t *testing.T, machineSet *machinev1.MachineSet) *machinev1.VSphereMachineProviderSpec {
	t.Helper()
	require.NotNil(t, machineSet.Spec.Template.Spec.ProviderSpec.Value, "MachineSet %s has no provider spec", machineSet.Name)
	provider := &machinev1.VSphereMachineProviderSpec{}
	require.NoError(t, json.Unmarshal(machineSet.Spec.Template.Spec.ProviderSpec.Value.Raw, provider), "cannot decode provider spec for MachineSet %s", machineSet.Name)
	return provider
}

func nodeIsReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
