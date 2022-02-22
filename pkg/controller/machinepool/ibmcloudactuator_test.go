package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	machineapi "github.com/openshift/api/machine/v1beta1"
	ibmcloudprovider "github.com/openshift/cluster-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	mockibm "github.com/openshift/hive/pkg/ibmclient/mock"
)

func TestIBMCloudActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool:              testIBMCloudPool(),
			expectedMachineSetReplicas: map[string]int64{
				generateIBMCloudMachineSetName("worker", "1"): 1,
				generateIBMCloudMachineSetName("worker", "2"): 1,
				generateIBMCloudMachineSetName("worker", "3"): 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			ibmcloudClient := mockibm.NewMockAPI(mockCtrl)
			ibmcloudClient.EXPECT().GetVPCZonesForRegion(gomock.Any(), "us-east").Return([]string{"us-east-1", "us-east-2", "us-east-3"}, nil).AnyTimes()

			actuator := &IBMCloudActuator{
				logger:    log.WithField("actuator", "ibmcloudactuator_test"),
				ibmClient: ibmcloudClient,
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test cast")
				validateIBMCloudMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func validateIBMCloudMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		ibmCloudProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*ibmcloudprovider.IBMCloudMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to ibmcloud provider spec") {
			assert.Equal(t, "bx2-4x16", ibmCloudProvider.Profile, "unexpected InstanceType")
		}
	}
}

func testIBMCloudPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		IBMCloud: &hivev1ibmcloud.MachinePool{
			InstanceType: "bx2-4x16",
		},
	}
	return p
}

func testIBMCloudClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		IBMCloud: &hivev1ibmcloud.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "ibmcloud-credentials",
			},
			AccountID:      "accountid",
			CISInstanceCRN: "cisinstancecrn",
			Region:         "us-east",
		},
	}
	return cd
}

func generateIBMCloudMachineSetName(leaseChar, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, leaseChar, zone)
}
