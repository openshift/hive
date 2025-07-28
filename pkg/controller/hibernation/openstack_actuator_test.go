package hibernation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	"github.com/openshift/hive/pkg/openstackclient"
	mockopenstackclient "github.com/openshift/hive/pkg/openstackclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestOpenStackCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.OpenStack = &hivev1openstack.Platform{}
	}).Build()
	actuator := openstackActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestOpenStackStopMachines(t *testing.T) {
	tests := []struct {
		name        string
		instances   map[string]int
		setupClient func(*testing.T, *mockopenstackclient.MockClient)
		expectErr   bool
	}{
		{
			name:      "stop no running instances",
			instances: map[string]int{}, 
		},
		{
			name:      "stop running instances",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Expect snapshot creation
				c.EXPECT().CreateServerSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return("snapshot-1", nil)

				// Expect image status checks
				activeImg := &images.Image{}
				activeImg.Status = "active"
				c.EXPECT().GetImage(gomock.Any(), gomock.Any()).Return(activeImg, nil).AnyTimes()

				// Expect instance deletion
				c.EXPECT().DeleteServer(gomock.Any(), gomock.Any()).Times(2).Return(nil)

				// Network operations
				setupOpenStackNetworkOps(c)

				// Tag operations for hibernation (capture tags)
				setupOpenStackTagOps(c)

				// OVERRIDE the default ListServers to simulate cleanup progression
				gomock.InOrder(
					// Initial calls: return instances
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
						{ID: "testinfra-ACTIVE-0", Name: "testinfra-ACTIVE-0", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
						{ID: "testinfra-ACTIVE-1", Name: "testinfra-ACTIVE-1", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
					}, nil).Times(2),

					// First cleanup poll: still there
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
						{ID: "testinfra-ACTIVE-0", Name: "testinfra-ACTIVE-0"},
						{ID: "testinfra-ACTIVE-1", Name: "testinfra-ACTIVE-1"},
					}, nil).Times(1),

					// Subsequent cleanup polls: instances gone
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).AnyTimes(),
				)
			},
		},
		{
			name:      "unable to list servers",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return(nil, errors.New("cannot list servers"))
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)
			if !test.expectErr {
				setupOpenStackClientInstances(openstackClient, test.instances)
			}
			if test.setupClient != nil {
				test.setupClient(t, openstackClient)
			}

			actuator := testOpenStackActuator(openstackClient)
			c := testfake.NewFakeClientBuilder().Build()
			err := actuator.StopMachines(testOpenStackClusterDeployment(), c, log.New())

			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestOpenStackStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		instances   map[string]int
		setupClient func(*testing.T, *mockopenstackclient.MockClient)
		withSecret  bool
	}{
		{
			name:      "start with existing instances",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Setup ListServers for existing instances check
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "testinfra-ACTIVE-0", Name: "testinfra-ACTIVE-0", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
					{ID: "testinfra-ACTIVE-1", Name: "testinfra-ACTIVE-1", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
				}, nil).AnyTimes()
			},
		},
		{
			name:       "start from hibernation config with quota validation",
			instances:  map[string]int{}, // No existing instances
			withSecret: true,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Setup quota checking mocks (sufficient resources)
				setupOpenStackQuotaMocks(c)

				// Expect image validation
				activeImg := &images.Image{}
				activeImg.Status = "active"
				c.EXPECT().GetImage(gomock.Any(), gomock.Any()).Return(activeImg, nil).AnyTimes()

				// STEP 1: First few calls should return NO instances (so creation happens)
				// STEP 2: After creation, return the created instances
				gomock.InOrder(
					// First call: no existing instances (for existing instance check)
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1),

					// Second call: still no instances (for missing instances check)
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1),

					// After creation: return the newly created instances
					c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
						{ID: "new-testinfra-master-0", Name: "testinfra-master-0", Status: "ACTIVE"},
						{ID: "new-testinfra-worker-0", Name: "testinfra-worker-0", Status: "ACTIVE"},
					}, nil).AnyTimes(),
				)

				// Expect server creation
				c.EXPECT().CreateServerFromOpts(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
					func(ctx context.Context, opts *openstackclient.ServerCreateOpts) (*servers.Server, error) {
						return &servers.Server{
							ID:     fmt.Sprintf("new-%s", opts.Name),
							Name:   opts.Name,
							Status: "BUILD",
						}, nil
					})

				// Expect tag restoration after server creation
				c.EXPECT().SetServerTags(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)

				// Expect status checks for waiting
				activeServer := &servers.Server{Status: "ACTIVE"}
				c.EXPECT().GetServer(gomock.Any(), gomock.Any()).Return(activeServer, nil).AnyTimes()
			},
		},
		{
			name:       "start from hibernation config - insufficient quota",
			instances:  map[string]int{}, // No existing instances
			withSecret: true,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Setup quota checking mocks (insufficient resources)
				setupOpenStackQuotaMocksInsufficient(c)

				// First call: no existing instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)

			// Only call setupOpenStackClientInstances for simple tests, not hibernation config tests
			if !test.withSecret {
				setupOpenStackClientInstances(openstackClient, test.instances)
			}

			if test.setupClient != nil {
				test.setupClient(t, openstackClient)
			}

			var c client.Client
			if test.withSecret {
				c = testfake.NewFakeClientBuilder().WithRuntimeObjects(testHibernationSecretWithMetadata()).Build()
			} else {
				c = testfake.NewFakeClientBuilder().Build()
			}

			actuator := testOpenStackActuator(openstackClient)
			err := actuator.StartMachines(testOpenStackClusterDeployment(), c, log.New())

			// Check expectations based on test case
			if test.name == "start from hibernation config - insufficient quota" {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), "resource validation failed")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// Test quota checking functionality separately
func TestOpenStackQuotaValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func(*testing.T, *mockopenstackclient.MockClient)
		expectErr   bool
	}{
		{
			name: "sufficient quotas",
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				setupOpenStackQuotaMocks(c)
			},
			expectErr: false,
		},
		{
			name: "insufficient quotas",
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				setupOpenStackQuotaMocksInsufficient(c)
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)
			test.setupClient(t, openstackClient)

			actuator := testOpenStackActuator(openstackClient)

			// Create test instances for validation
			instances := []OpenStackInstanceConfig{
				{Name: "test-master-0", Flavor: "flavor-1"},
				{Name: "test-worker-0", Flavor: "flavor-2"},
			}

			err := actuator.validateRestoreResources(openstackClient, instances, log.New())

			if test.expectErr {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), "insufficient")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func testOpenStackActuator(openstackClient openstackclient.Client) *openstackActuator {
	return &openstackActuator{
		openstackClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (openstackclient.Client, error) {
			return openstackClient, nil
		},
	}
}

func setupOpenStackClientInstances(openstackClient *mockopenstackclient.MockClient, statuses map[string]int) {
	var allServers []servers.Server
	for status, count := range statuses {
		for i := 0; i < count; i++ {
			serverName := fmt.Sprintf("%s-%d", status, i)
			server := servers.Server{
				ID:     fmt.Sprintf("testinfra-%s", serverName),
				Name:   fmt.Sprintf("testinfra-%s", serverName),
				Status: status,
				Flavor: map[string]interface{}{"id": "flavor-1"},
			}
			allServers = append(allServers, server)
		}
	}

	// Only setup ListServers for simple tests (empty instances)
	if len(allServers) == 0 {
		openstackClient.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return(allServers, nil).AnyTimes()
	}

	// Setup GetServer calls
	for _, server := range allServers {
		openstackClient.EXPECT().GetServer(gomock.Any(), server.ID).Return(&server, nil).AnyTimes()
	}
}

func setupOpenStackNetworkOps(openstackClient *mockopenstackclient.MockClient) {
	// Network operations
	network := &openstackclient.Network{
		ID:   "network-1",
		Name: "testinfra-openshift",
	}
	openstackClient.EXPECT().GetNetworkByName(gomock.Any(), "testinfra-openshift").Return(network, nil).AnyTimes()

	// Port listing
	ports := []openstackclient.Port{
		{ID: "port-1", Name: "testinfra-ACTIVE-0"},
		{ID: "port-2", Name: "testinfra-ACTIVE-1"},
	}
	openstackClient.EXPECT().ListPorts(gomock.Any()).Return(ports, nil).AnyTimes()

	// Security groups
	openstackClient.EXPECT().GetServerSecurityGroups(gomock.Any(), gomock.Any()).Return([]string{"default"}, nil).AnyTimes()
}

// Setup tag operations for tests
func setupOpenStackTagOps(openstackClient *mockopenstackclient.MockClient) {
	// Expect tag retrieval during hibernation (capture tags)
	masterTags := []string{"openshiftClusterID=testinfra"}
	workerTags := []string{"cluster-api-provider-openstack", "openshift-machine-api-testinfra", "openshiftClusterID=testinfra"}

	// Mock tag calls for different servers
	openstackClient.EXPECT().GetServerTags(gomock.Any(), "testinfra-ACTIVE-0").Return(masterTags, nil).AnyTimes()
	openstackClient.EXPECT().GetServerTags(gomock.Any(), "testinfra-ACTIVE-1").Return(workerTags, nil).AnyTimes()
}

// Setup quota mocks with sufficient resources
func setupOpenStackQuotaMocks(openstackClient *mockopenstackclient.MockClient) {
	// Mock quotas - plenty of resources available
	quotas := &openstackclient.ComputeQuotas{
		Instances: 20,
		Cores:     40,
		RAM:       81920,
	}
	openstackClient.EXPECT().GetComputeQuotas(gomock.Any()).Return(quotas, nil).AnyTimes()

	// Mock usage - minimal usage
	usage := &openstackclient.ComputeUsage{
		InstancesUsed: 0,
		CoresUsed:     0,
		RAMUsed:       0,
	}
	openstackClient.EXPECT().GetComputeUsage(gomock.Any()).Return(usage, nil).AnyTimes()

	// Mock flavor details for our test flavors
	flavorDetails := &openstackclient.FlavorDetails{
		ID:    "flavor-1",
		Name:  "m1.small",
		VCPUs: 2,
		RAM:   2048, // 2GB in MB
		Disk:  10,
	}
	openstackClient.EXPECT().GetFlavorDetails(gomock.Any(), "flavor-1").Return(flavorDetails, nil).AnyTimes()

	// Also mock flavor-2 for worker
	flavorDetails2 := &openstackclient.FlavorDetails{
		ID:    "flavor-2",
		Name:  "m1.medium",
		VCPUs: 4,
		RAM:   4096, // 4GB in MB
		Disk:  20,
	}
	openstackClient.EXPECT().GetFlavorDetails(gomock.Any(), "flavor-2").Return(flavorDetails2, nil).AnyTimes()
}

// Setup quota mocks with insufficient resources
func setupOpenStackQuotaMocksInsufficient(openstackClient *mockopenstackclient.MockClient) {
	// Mock quotas - very limited resources
	quotas := &openstackclient.ComputeQuotas{
		Instances: 2,
		Cores:     4,    // Only 4 vCPUs total
		RAM:       4096, // Only 4GB total
	}
	openstackClient.EXPECT().GetComputeQuotas(gomock.Any()).Return(quotas, nil).AnyTimes()

	// Mock usage - most resources already used
	usage := &openstackclient.ComputeUsage{
		InstancesUsed: 1,
		CoresUsed:     3,    // 3 out of 4 vCPUs used
		RAMUsed:       3072, // 3GB out of 4GB used
	}
	openstackClient.EXPECT().GetComputeUsage(gomock.Any()).Return(usage, nil).AnyTimes()

	// Mock flavor details - will show insufficient resources
	flavorDetails1 := &openstackclient.FlavorDetails{
		ID:    "flavor-1",
		Name:  "m1.small",
		VCPUs: 2,
		RAM:   2048, // 2GB in MB
		Disk:  10,
	}
	openstackClient.EXPECT().GetFlavorDetails(gomock.Any(), "flavor-1").Return(flavorDetails1, nil).AnyTimes()

	// Also mock flavor-2 for worker
	flavorDetails2 := &openstackclient.FlavorDetails{
		ID:    "flavor-2",
		Name:  "m1.medium",
		VCPUs: 4,
		RAM:   4096, // 4GB in MB
		Disk:  20,
	}
	openstackClient.EXPECT().GetFlavorDetails(gomock.Any(), "flavor-2").Return(flavorDetails2, nil).AnyTimes()
}

func testOpenStackClusterDeployment() *hivev1.ClusterDeployment {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder("testns", "testopenstackcluster", scheme)
	return cdBuilder.Build(
		testcd.WithOpenStackPlatform(&hivev1openstack.Platform{Cloud: "openstack"}),
		testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: "testinfra"}),
		testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
	)
}

// hibernation secret with complete metadata and tags
func testHibernationSecretWithMetadata() *corev1.Secret {
	config := []OpenStackInstanceConfig{
		{
			Name:               "testinfra-master-0",
			Flavor:             "flavor-1",
			PortID:             "port-1",
			SnapshotID:         "snapshot-1",
			SecurityGroups:     []string{"default"},
			ClusterID:          "testinfra",
			NetworkID:          "network-1",
			OpenshiftClusterID: "testinfra",
			Metadata: map[string]string{
				"Name":               "testinfra-master",
				"openshiftClusterID": "testinfra",
			},
			Tags: []string{"openshiftClusterID=testinfra"},
		},
		{
			Name:               "testinfra-worker-0",
			Flavor:             "flavor-2",
			PortID:             "port-2",
			SnapshotID:         "snapshot-2",
			SecurityGroups:     []string{"default"},
			ClusterID:          "testinfra",
			NetworkID:          "network-1",
			OpenshiftClusterID: "testinfra",
			Metadata: map[string]string{
				"Name":               "testinfra-worker",
				"openshiftClusterID": "testinfra",
			},
			Tags: []string{"cluster-api-provider-openstack", "openshift-machine-api-testinfra", "openshiftClusterID=testinfra"},
		},
	}

	configData, _ := json.Marshal(config)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testopenstackcluster-hibernation-config",
			Namespace: "testns",
		},
		Data: map[string][]byte{
			"hibernation-config": configData,
		},
	}
}
