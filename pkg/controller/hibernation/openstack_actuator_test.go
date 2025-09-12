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
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
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
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// No instances found
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)
			},
		},
		{
			name:      "stop running instances with pause/unpause flow",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Initial server discovery
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "server-1", Name: "testinfra-master-0", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
					{ID: "server-2", Name: "testinfra-worker-0", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
				}, nil).Times(1)

				// Pause all instances
				c.EXPECT().PauseServer(gomock.Any(), "server-1").Return(nil).Times(1)
				c.EXPECT().PauseServer(gomock.Any(), "server-2").Return(nil).Times(1)

				// Create snapshots (timestamp will be consistent)
				c.EXPECT().CreateServerSnapshot(gomock.Any(), "server-1", gomock.Any()).Return("snapshot-1", nil).Times(1)
				c.EXPECT().CreateServerSnapshot(gomock.Any(), "server-2", gomock.Any()).Return("snapshot-2", nil).Times(1)

				// Wait for snapshots to complete
				activeImg := &images.Image{Status: "active"}
				c.EXPECT().GetImage(gomock.Any(), "snapshot-1").Return(activeImg, nil).AnyTimes()
				c.EXPECT().GetImage(gomock.Any(), "snapshot-2").Return(activeImg, nil).AnyTimes()

				// Network operations for saving configuration
				setupOpenStackNetworkOps(c)

				// Security groups for configuration
				c.EXPECT().GetServerSecurityGroupNames(gomock.Any(), gomock.Any()).Return([]string{"default"}, nil).AnyTimes()

				// Tags for configuration
				c.EXPECT().GetServerTags(gomock.Any(), gomock.Any()).Return([]string{"tag1", "tag2"}, nil).AnyTimes()

				// Delete instances
				c.EXPECT().DeleteServer(gomock.Any(), "server-1").Return(nil).Times(1)
				c.EXPECT().DeleteServer(gomock.Any(), "server-2").Return(nil).Times(1)

				// Wait for cleanup - first check shows deleting, second shows empty
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "server-1", Name: "testinfra-master-0", Status: "DELETING"},
				}, nil).Times(1)
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)

				// Cleanup old snapshots
				c.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return([]images.Image{}, nil).Times(1)
			},
		},
		{
			name:      "pause failure should unpause previous instances",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Initial server discovery
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "server-1", Name: "testinfra-master-0", Status: "ACTIVE"},
					{ID: "server-2", Name: "testinfra-worker-0", Status: "ACTIVE"},
				}, nil).Times(1)

				// First pause succeeds
				c.EXPECT().PauseServer(gomock.Any(), "server-1").Return(nil).Times(1)
				// Second pause fails
				c.EXPECT().PauseServer(gomock.Any(), "server-2").Return(errors.New("pause failed")).Times(1)

				// Should unpause the first server due to failure
				c.EXPECT().UnpauseServer(gomock.Any(), "server-1").Return(nil).Times(1)
			},
			expectErr: true,
		},
		{
			name:      "snapshot creation with old snapshot cleanup",
			instances: map[string]int{"ACTIVE": 1},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Initial server discovery
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "server-1", Name: "testinfra-master-0", Status: "ACTIVE", Flavor: map[string]interface{}{"id": "flavor-1"}},
				}, nil).Times(1)

				// Pause
				c.EXPECT().PauseServer(gomock.Any(), "server-1").Return(nil).Times(1)

				// Snapshot creation
				c.EXPECT().CreateServerSnapshot(gomock.Any(), "server-1", gomock.Any()).Return("snapshot-new", nil).Times(1)

				// Wait for snapshot
				activeImg := &images.Image{Status: "active"}
				c.EXPECT().GetImage(gomock.Any(), "snapshot-new").Return(activeImg, nil).AnyTimes()

				// Network operations
				setupOpenStackNetworkOps(c)

				// Security groups for configuration
				c.EXPECT().GetServerSecurityGroupNames(gomock.Any(), gomock.Any()).Return([]string{"default"}, nil).AnyTimes()

				// Tags for configuration
				c.EXPECT().GetServerTags(gomock.Any(), gomock.Any()).Return([]string{"tag1", "tag2"}, nil).AnyTimes()

				// Delete instance
				c.EXPECT().DeleteServer(gomock.Any(), "server-1").Return(nil).Times(1)

				// Cleanup wait
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)

				// Old snapshot cleanup - return old snapshots and delete them
				oldSnapshots := []images.Image{
					{ID: "old-snap-1", Name: "testinfra-master-0-hibernation-20240101"},
					{ID: "old-snap-2", Name: "testinfra-worker-0-hibernation-20240102"},
				}
				c.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(oldSnapshots, nil).Times(1)
				c.EXPECT().DeleteImage(gomock.Any(), "old-snap-1").Return(nil).Times(1)
				c.EXPECT().DeleteImage(gomock.Any(), "old-snap-2").Return(nil).Times(1)
			},
		},
		{
			name:      "unable to list servers",
			instances: map[string]int{"ACTIVE": 1},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return(nil, errors.New("cannot list servers")).Times(1)
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)
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
		expectErr   bool
	}{
		{
			name:      "start with existing instances - clear hibernation config",
			instances: map[string]int{"ACTIVE": 2},
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Check existing instances - return enough instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "server-1", Name: "testinfra-master-0", Status: "ACTIVE"},
					{ID: "server-2", Name: "testinfra-worker-0", Status: "ACTIVE"},
				}, nil).Times(1)
			},
			withSecret: true, // Has hibernation config but instances already exist
		},
		{
			name:       "start from hibernation config - successful restoration",
			instances:  map[string]int{}, // No existing instances
			withSecret: true,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// First check: no existing instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)

				// Second check: still no instances (for missing instances check)
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)

				// Snapshot validation
				activeImg := &images.Image{Status: "active"}
				c.EXPECT().GetImage(gomock.Any(), "snapshot-1").Return(activeImg, nil).Times(1)
				c.EXPECT().GetImage(gomock.Any(), "snapshot-2").Return(activeImg, nil).Times(1)

				// Server creation
				c.EXPECT().CreateServerFromOpts(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
					func(ctx context.Context, opts *servers.CreateOpts) (*servers.Server, error) {
						return &servers.Server{
							ID:     fmt.Sprintf("new-%s", opts.Name),
							Name:   opts.Name,
							Status: "BUILD",
						}, nil
					})

				// Wait for instances to be active - return active instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "new-testinfra-master-0", Name: "testinfra-master-0", Status: "ACTIVE"},
					{ID: "new-testinfra-worker-0", Name: "testinfra-worker-0", Status: "ACTIVE"},
				}, nil).AnyTimes()

				// FIXED: Add tag restoration expectations
				c.EXPECT().SetServerTags(gomock.Any(), "new-testinfra-master-0", []string{"tag1", "tag2"}).Return(nil).Times(1)
				c.EXPECT().SetServerTags(gomock.Any(), "new-testinfra-worker-0", []string{"tag1", "tag2"}).Return(nil).Times(1)

				// Cleanup snapshots after successful restoration
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-1").Return(nil).Times(1)
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-2").Return(nil).Times(1)
			},
		},
		{
			name:       "start from hibernation - some instances already exist",
			instances:  map[string]int{},
			withSecret: true,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// First check: one instance already exists
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "existing-1", Name: "testinfra-master-0", Status: "ACTIVE"},
				}, nil).Times(1)

				// Second check for missing instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "existing-1", Name: "testinfra-master-0", Status: "ACTIVE"},
				}, nil).Times(1)

				// Only create the missing worker instance
				activeImg := &images.Image{Status: "active"}
				c.EXPECT().GetImage(gomock.Any(), "snapshot-2").Return(activeImg, nil).Times(1)

				c.EXPECT().CreateServerFromOpts(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
					func(ctx context.Context, opts *servers.CreateOpts) (*servers.Server, error) {
						// Should only create the worker instance
						assert.Equal(t, "testinfra-worker-0", opts.Name)
						return &servers.Server{
							ID:     "new-testinfra-worker-0",
							Name:   opts.Name,
							Status: "BUILD",
						}, nil
					})

				// Wait for all instances active
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "existing-1", Name: "testinfra-master-0", Status: "ACTIVE"},
					{ID: "new-testinfra-worker-0", Name: "testinfra-worker-0", Status: "ACTIVE"},
				}, nil).AnyTimes()

				// FIXED: Add specific tag restoration expectations for both instances
				c.EXPECT().SetServerTags(gomock.Any(), "existing-1", []string{"tag1", "tag2"}).Return(nil).Times(1)
				c.EXPECT().SetServerTags(gomock.Any(), "new-testinfra-worker-0", []string{"tag1", "tag2"}).Return(nil).Times(1)

				// Cleanup snapshots
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-1").Return(nil).Times(1)
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-2").Return(nil).Times(1)
			},
		},
		{
			name:       "start from hibernation - snapshot missing error",
			instances:  map[string]int{},
			withSecret: true,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// No existing instances
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)

				// First snapshot exists, second is missing
				activeImg := &images.Image{Status: "active"}
				c.EXPECT().GetImage(gomock.Any(), "snapshot-1").Return(activeImg, nil).Times(1)
				c.EXPECT().GetImage(gomock.Any(), "snapshot-2").Return(nil, errors.New("snapshot not found")).Times(1)

				// Code still tries to create the first instance (snapshot-1 was valid)
				c.EXPECT().CreateServerFromOpts(gomock.Any(), gomock.Any()).Times(1).Return(&servers.Server{
					ID: "new-master", Name: "testinfra-master-0", Status: "BUILD",
				}, nil)

				// It will try to wait for instances to be active
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
					{ID: "new-master", Name: "testinfra-master-0", Status: "ACTIVE"},
					{ID: "fake-worker", Name: "testinfra-worker-0", Status: "ACTIVE"},
				}, nil).AnyTimes()

				// FIXED: Add tag restoration expectations for both instances
				c.EXPECT().SetServerTags(gomock.Any(), "new-master", []string{"tag1", "tag2"}).Return(nil).Times(1)
				c.EXPECT().SetServerTags(gomock.Any(), "fake-worker", []string{"tag1", "tag2"}).Return(nil).Times(1)

				// FIXED: Add both snapshot cleanup calls
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-1").Return(nil).Times(1)
				c.EXPECT().DeleteImage(gomock.Any(), "snapshot-2").Return(nil).Times(1)
			},
			expectErr: false, // Code continues even with partial failure
		},
		{
			name:       "no hibernation config and no instances",
			instances:  map[string]int{},
			withSecret: false,
			setupClient: func(t *testing.T, c *mockopenstackclient.MockClient) {
				// Check for existing instances - none found
				c.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)

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

			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestOpenStackMachinesRunning(t *testing.T) {
	tests := []struct {
		name            string
		servers         []servers.Server
		expectedRunning bool
		expectedNames   []string
	}{
		{
			name:            "no instances found",
			servers:         []servers.Server{},
			expectedRunning: false,
			expectedNames:   []string{"no instances found"},
		},
		{
			name: "all instances running",
			servers: []servers.Server{
				{ID: "1", Name: "testinfra-master-0", Status: "ACTIVE"},
				{ID: "2", Name: "testinfra-worker-0", Status: "ACTIVE"},
			},
			expectedRunning: true,
			expectedNames:   []string{},
		},
		{
			name: "instances being deleted",
			servers: []servers.Server{
				{ID: "1", Name: "testinfra-master-0", Status: "DELETING"},
				{ID: "2", Name: "testinfra-worker-0", Status: "DELETING"},
			},
			expectedRunning: false,
			expectedNames:   []string{"instances-being-deleted"},
		},
		{
			name: "mixed states",
			servers: []servers.Server{
				{ID: "1", Name: "testinfra-master-0", Status: "ACTIVE"},
				{ID: "2", Name: "testinfra-worker-0", Status: "SHUTOFF"},
			},
			expectedRunning: true, // At least one is running
			expectedNames:   []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)
			openstackClient.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return(test.servers, nil).Times(1)

			actuator := testOpenStackActuator(openstackClient)
			c := testfake.NewFakeClientBuilder().Build()

			running, names, err := actuator.MachinesRunning(testOpenStackClusterDeployment(), c, log.New())

			assert.Nil(t, err)
			assert.Equal(t, test.expectedRunning, running)
			assert.Equal(t, test.expectedNames, names)
		})
	}
}

func TestOpenStackMachinesStopped(t *testing.T) {
	tests := []struct {
		name            string
		servers         []servers.Server
		expectedStopped bool
		expectedNames   []string
	}{
		{
			name:            "no instances found - stopped",
			servers:         []servers.Server{},
			expectedStopped: true,
			expectedNames:   nil,
		},
		{
			name: "instances still running",
			servers: []servers.Server{
				{ID: "1", Name: "testinfra-master-0", Status: "ACTIVE"},
				{ID: "2", Name: "testinfra-worker-0", Status: "ACTIVE"},
			},
			expectedStopped: false,
			expectedNames:   []string{"testinfra-master-0", "testinfra-worker-0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openstackClient := mockopenstackclient.NewMockClient(ctrl)
			openstackClient.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return(test.servers, nil).Times(1)

			actuator := testOpenStackActuator(openstackClient)
			c := testfake.NewFakeClientBuilder().Build()

			stopped, names, err := actuator.MachinesStopped(testOpenStackClusterDeployment(), c, log.New())

			assert.Nil(t, err)
			assert.Equal(t, test.expectedStopped, stopped)
			assert.Equal(t, test.expectedNames, names)
		})
	}
}

// Helper functions

func testOpenStackActuator(openstackClient openstackclient.Client) *openstackActuator {
	return &openstackActuator{
		openstackClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (openstackclient.Client, error) {
			return openstackClient, nil
		},
	}
}

func setupOpenStackNetworkOps(openstackClient *mockopenstackclient.MockClient) {
	// Network operations
	network := &networks.Network{
		ID:     "network-1",
		Name:   "testinfra-openshift",
		Status: "ACTIVE",
	}
	openstackClient.EXPECT().GetNetworkByName(gomock.Any(), "testinfra-openshift").Return(network, nil).AnyTimes()

	// Port listing
	ports := []ports.Port{
		{ID: "port-1", Name: "testinfra-master-0"},
		{ID: "port-2", Name: "testinfra-worker-0"},
	}
	openstackClient.EXPECT().ListPorts(gomock.Any()).Return(ports, nil).AnyTimes()
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

// hibernation secret with complete metadata
func testHibernationSecretWithMetadata() *corev1.Secret {
	config := []OpenStackInstanceConfig{
		{
			Name:               "testinfra-master-0",
			Flavor:             "flavor-1",
			PortID:             "port-1",
			SnapshotID:         "snapshot-1",
			SnapshotName:       "testinfra-master-0-hibernation-20240101-120000",
			SecurityGroups:     []string{"default"},
			ClusterID:          "testinfra",
			NetworkID:          "network-1",
			OpenshiftClusterID: "testinfra",
			Metadata: map[string]string{
				"Name":               "testinfra-master",
				"openshiftClusterID": "testinfra",
			},
			Tags: []string{"tag1", "tag2"},
		},
		{
			Name:               "testinfra-worker-0",
			Flavor:             "flavor-2",
			PortID:             "port-2",
			SnapshotID:         "snapshot-2",
			SnapshotName:       "testinfra-worker-0-hibernation-20240101-120000",
			SecurityGroups:     []string{"default"},
			ClusterID:          "testinfra",
			NetworkID:          "network-1",
			OpenshiftClusterID: "testinfra",
			Metadata: map[string]string{
				"Name":               "testinfra-worker",
				"openshiftClusterID": "testinfra",
			},
			Tags: []string{"tag1", "tag2"},
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

func TestExactNameFiltering(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	openstackClient := mockopenstackclient.NewMockClient(ctrl)

	// Mock the exact name filtering call
	expectedSnapshots := []images.Image{
		{ID: "snapshot-1", Name: "testinfra-master-0-hibernation-20240101-120000"},
		{ID: "snapshot-2", Name: "testinfra-worker-0-hibernation-20240101-120000"},
	}

	openstackClient.EXPECT().ListImages(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, opts *images.ListOpts) ([]images.Image, error) {
			// Verify the "in:" filter is being used
			expectedFilter := "in:testinfra-master-0-hibernation-20240101-120000,testinfra-worker-0-hibernation-20240101-120000"
			assert.Equal(t, expectedFilter, opts.Name)
			return expectedSnapshots, nil
		}).Times(1)

	actuator := testOpenStackActuator(openstackClient)
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(testHibernationSecretWithMetadata()).Build()

	cd := testOpenStackClusterDeployment()
	snapshots, err := actuator.findHibernationSnapshotsByExactNames(cd, c, openstackClient, "testinfra", log.New())

	assert.Nil(t, err)
	assert.Equal(t, 2, len(snapshots))
	assert.Equal(t, "snapshot-1", snapshots[0].ID)
}

func TestSnapshotCleanupWithExactFiltering(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	openstackClient := mockopenstackclient.NewMockClient(ctrl)

	// Mock exact name filtering returning both old and new snapshots
	allSnapshots := []images.Image{
		{ID: "snapshot-new", Name: "testinfra-master-0-hibernation-20240201-120000"}, // Current
		{ID: "snapshot-old", Name: "testinfra-master-0-hibernation-20240101-120000"}, // Old
	}

	openstackClient.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(allSnapshots, nil).Times(1)

	// Should only delete the old snapshot
	openstackClient.EXPECT().DeleteImage(gomock.Any(), "snapshot-old").Return(nil).Times(1)

	actuator := testOpenStackActuator(openstackClient)
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(testHibernationSecretWithMetadata()).Build()

	cd := testOpenStackClusterDeployment()
	err := actuator.cleanupOldSnapshotsAfterDeletion(cd, c, openstackClient, "testinfra", []string{"snapshot-new"}, log.New())

	assert.Nil(t, err)
}

func TestStartMachinesWithTagRestoration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	openstackClient := mockopenstackclient.NewMockClient(ctrl)

	// Initial checks - no existing instances
	openstackClient.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{}, nil).Times(2)

	// Snapshot validation
	activeImg := &images.Image{Status: "active"}
	openstackClient.EXPECT().GetImage(gomock.Any(), "snapshot-1").Return(activeImg, nil).Times(1)
	openstackClient.EXPECT().GetImage(gomock.Any(), "snapshot-2").Return(activeImg, nil).Times(1)

	// Server creation
	openstackClient.EXPECT().CreateServerFromOpts(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(ctx context.Context, opts *servers.CreateOpts) (*servers.Server, error) {
			return &servers.Server{
				ID:     fmt.Sprintf("new-%s", opts.Name),
				Name:   opts.Name,
				Status: "BUILD",
			}, nil
		})

	// Wait for instances to be active
	openstackClient.EXPECT().ListServers(gomock.Any(), gomock.Any()).Return([]servers.Server{
		{ID: "new-testinfra-master-0", Name: "testinfra-master-0", Status: "ACTIVE"},
		{ID: "new-testinfra-worker-0", Name: "testinfra-worker-0", Status: "ACTIVE"},
	}, nil).AnyTimes()

	// Tag restoration calls
	openstackClient.EXPECT().SetServerTags(gomock.Any(), "new-testinfra-master-0", []string{"tag1", "tag2"}).Return(nil).Times(1)
	openstackClient.EXPECT().SetServerTags(gomock.Any(), "new-testinfra-worker-0", []string{"tag1", "tag2"}).Return(nil).Times(1)

	// Cleanup snapshots
	openstackClient.EXPECT().DeleteImage(gomock.Any(), "snapshot-1").Return(nil).Times(1)
	openstackClient.EXPECT().DeleteImage(gomock.Any(), "snapshot-2").Return(nil).Times(1)

	actuator := testOpenStackActuator(openstackClient)
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(testHibernationSecretWithMetadata()).Build()

	err := actuator.StartMachines(testOpenStackClusterDeployment(), c, log.New())
	assert.Nil(t, err)
}

func TestPauseUnpauseFlow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	openstackClient := mockopenstackclient.NewMockClient(ctrl)

	servers := []*servers.Server{
		{ID: "server-1", Name: "testinfra-master-0", Status: "ACTIVE"},
		{ID: "server-2", Name: "testinfra-worker-0", Status: "ACTIVE"},
	}

	// Test successful pause
	openstackClient.EXPECT().PauseServer(gomock.Any(), "server-1").Return(nil).Times(1)
	openstackClient.EXPECT().PauseServer(gomock.Any(), "server-2").Return(nil).Times(1)

	actuator := testOpenStackActuator(openstackClient)
	err := actuator.pauseInstances(openstackClient, servers, log.New())
	assert.Nil(t, err)

	// Test unpause
	openstackClient.EXPECT().UnpauseServer(gomock.Any(), "server-1").Return(nil).Times(1)
	openstackClient.EXPECT().UnpauseServer(gomock.Any(), "server-2").Return(nil).Times(1)

	actuator.unpauseInstances(openstackClient, servers, log.New())
}
