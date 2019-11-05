package syncmachineset

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gcpprovider "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	hiveapi "github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
	"github.com/openshift/hive/pkg/constants"
	mockactuator "github.com/openshift/hive/pkg/controller/syncmachineset/mock"
	"github.com/openshift/hive/pkg/resource"
)

const (
	testNamespace   = "default"
	testName        = "foo"
	sshKeySecret    = "foo-ssh-key"
	sshKeySecretKey = "ssh-publickey"
	gcpCredsSecret  = "foo-gcp-creds"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

type createdMachineSetInfo struct {
	name         string
	namespace    string
	kind         string
	replicas     int
	instanceType string
}
type createdSyncSetInfo struct {
	name      string
	namespace string
	resources []createdMachineSetInfo
	syncSet   *hivev1.SyncSet
}

type fakeApplier struct {
	createdSyncSet createdSyncSetInfo
}

// ApplyRuntimeObject expects to be called once with the created SyncSet filled out. It will inspect the contents
// of the SyncSet and record some of the data in the SyncSet.
func (a *fakeApplier) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error) {
	ss := obj.(*hivev1.SyncSet)
	created := createdSyncSetInfo{
		name:      ss.Name,
		namespace: ss.Namespace,
	}

	created.syncSet = ss

	for _, raw := range ss.Spec.Resources {
		ms, ok := raw.Object.(*machineapi.MachineSet)
		if ok {
			gcpProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*gcpprovider.GCPMachineProviderSpec)
			if ok {
				cr := createdMachineSetInfo{
					name:         ms.Name,
					namespace:    ms.Namespace,
					kind:         ms.Kind,
					replicas:     int(*ms.Spec.Replicas),
					instanceType: gcpProvider.MachineType,
				}
				created.resources = append(created.resources, cr)
			}
		}
	}

	a.createdSyncSet = created

	return "", nil
}

type generateMachineSetsResponse struct {
	machineSets []*machineapi.MachineSet
	err         error
}

func TestSyncMachineSetReconcile(t *testing.T) {
	hiveapi.AddToScheme(scheme.Scheme)
	hivev1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                string
		existingObjects     []runtime.Object
		generateMSResponse  generateMachineSetsResponse
		expectedMachineSets []createdMachineSetInfo
		expectErr           bool
		skipGenerateCall    bool
		validate            func(*testing.T, client.Client, []createdMachineSetInfo, createdSyncSetInfo)
	}{
		{
			name: "create sync set",
			existingObjects: []runtime.Object{
				testClusterDeployment(),
				testClusterSecret(),
			},
			expectedMachineSets: []createdMachineSetInfo{
				{
					name:         "machineSetA",
					replicas:     3,
					instanceType: "gcpInstanceType",
				},
			},
			generateMSResponse: generateMachineSetsResponse{
				machineSets: []*machineapi.MachineSet{
					testMachineSet("machineSetA", 3, "gcpInstanceType"),
				},
			},
		},
		{
			name: "no machine pool",
			existingObjects: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Compute = []hivev1.MachinePool{}
					return cd
				}(),
				testClusterSecret(),
			},
			expectedMachineSets: []createdMachineSetInfo{},
			generateMSResponse:  generateMachineSetsResponse{},
		},
		{
			name: "two machine pools",
			existingObjects: []runtime.Object{
				testClusterDeployment(),
				testClusterSecret(),
			},
			generateMSResponse: generateMachineSetsResponse{
				machineSets: []*machineapi.MachineSet{
					testMachineSet("machineset1", 2, "instanceTypeA"),
					testMachineSet("machineset2", 3, "instanceTypeB"),
				},
			},
			expectedMachineSets: []createdMachineSetInfo{
				{
					name:         "machineset1",
					replicas:     2,
					instanceType: "instanceTypeA",
				},
				{
					name:         "machineset2",
					replicas:     3,
					instanceType: "instanceTypeB",
				},
			},
		},
		{
			name: "set owner reference",
			existingObjects: []runtime.Object{
				testClusterDeployment(),
				testClusterSecret(),
			},
			validate: func(t *testing.T, c client.Client, expectedMS []createdMachineSetInfo, createdSS createdSyncSetInfo) {
				cd := getClusterDeployment(c)
				assert.NotNil(t, cd)

				foundOwnerReference := false
				for _, or := range createdSS.syncSet.OwnerReferences {
					if or.Name == cd.Name && or.Kind == "ClusterDeployment" {
						foundOwnerReference = true
						break
					}
				}
				assert.True(t, foundOwnerReference, "failed to find owner reference in sync set")
			},
		},
		{
			name: "skip with pause annotation",
			existingObjects: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Annotations = map[string]string{constants.SyncsetPauseAnnotation: "true"}
					return cd
				}(),
				testClusterSecret(),
			},
			validate: func(t *testing.T, c client.Client, expectedMS []createdMachineSetInfo, createdSS createdSyncSetInfo) {
				assert.Nil(t, createdSS.syncSet, "no call expected to created/update the syncset when paused")
			},
			skipGenerateCall: true,
		},
		{
			name: "error on generate error",
			existingObjects: []runtime.Object{
				testClusterDeployment(),
				testClusterSecret(),
			},
			generateMSResponse: generateMachineSetsResponse{
				err: fmt.Errorf("TEST ERROR"),
			},
			expectErr: true,
		},
		{
			name: "ignore unsupported platform",
			existingObjects: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Platform = hivev1.Platform{
						AWS: &hivev1aws.Platform{},
					}
					return cd
				}(),
				testClusterSecret(),
			},
			skipGenerateCall: true,
			validate: func(t *testing.T, c client.Client, expectedMS []createdMachineSetInfo, createdSS createdSyncSetInfo) {
				assert.Nil(t, createdSS.syncSet, "no sync sets when platform unsupported")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existingObjects...)

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockActuator := mockactuator.NewMockActuator(mockCtrl)

			applier := &fakeApplier{}

			logger := log.WithField("controller", "syncmachineset")

			rcd := &ReconcileSyncMachineSet{
				Client: fakeClient,
				scheme: scheme.Scheme,
				logger: logger,
				actuatorBuilder: func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (Actuator, error) {
					return mockActuator, nil
				},
				kubeCLI: applier,
			}

			if !test.skipGenerateCall {
				cd := getClusterDeployment(fakeClient)
				ic, err := rcd.generateInstallConfig(cd, logger)
				assert.NoError(t, err, "error creating fake installconfig for test")

				mockActuator.EXPECT().GenerateMachineSets(gomock.Eq(cd), gomock.Eq(ic), gomock.Any()).Return(
					test.generateMSResponse.machineSets, test.generateMSResponse.err,
				)
			}

			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})

			if test.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if test.validate != nil {
					test.validate(t, fakeClient, test.expectedMachineSets, applier.createdSyncSet)
				} else {
					validateSyncSet(t, test.expectedMachineSets, applier.createdSyncSet)
				}
			}
		})
	}
}

func validateSyncSet(t *testing.T, expectedMS []createdMachineSetInfo, createdSS createdSyncSetInfo) {
	assert.Equal(t, len(expectedMS), len(createdSS.resources), "unexpected number of machineset entries in syncset")
	for _, expected := range expectedMS {
		found := false

		for _, res := range createdSS.resources {
			if res.name == expected.name {
				found = true
				assert.Equal(t, expected.replicas, res.replicas, "mismatched replicas")
				assert.Equal(t, expected.instanceType, res.instanceType, "mismatched instance type")
			}
		}
		assert.True(t, found, "did not find machineset %s in syncset", expected.name)
	}
}

func testMachineSet(name string, replicas int, machineType string) *machineapi.MachineSet {
	ms := &machineapi.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: machineapi.MachineSetSpec{
			Replicas: pointer.Int32Ptr(int32(replicas)),
			Template: machineapi.MachineTemplateSpec{
				Spec: machineapi.MachineSpec{
					ProviderSpec: machineapi.ProviderSpec{
						Value: &runtime.RawExtension{Object: &gcpprovider.GCPMachineProviderSpec{
							MachineType: machineType,
						}},
					},
				},
			},
		},
	}
	return ms
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
			UID:       types.UID("1234"),
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ClusterName: testName,
			// Compute: contents of clusterDeployment don't really matter for this test.
			Platform: hivev1.Platform{
				GCP: &hivev1gcp.Platform{
					Region: "us-central1",
				},
			},
			Networking: hivev1.Networking{
				Type: hivev1.NetworkTypeOpenshiftSDN,
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				GCP: &hivev1gcp.PlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: gcpCredsSecret,
					},
				},
			},
			Installed: true,
		},
	}
}

func testClusterSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sshKeySecret,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			sshKeySecretKey: []byte("SOME SECRET DATA HERE"),
		},
	}
	return s
}

func getClusterDeployment(c client.Client) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: testNamespace}, cd); err != nil {
		cd = nil
	}
	return cd
}

func testGCPCredsSecret() *corev1.Secret {
	fakeCreds := `{
	"type": "service_account",
	"project_id": "test-project",
	"private_key_id": "somerandomhex",
	"private_key": "-----BEGIN PRIVATE KEY-----",
	"client_email": "test-account@test-project.iam.gserviceaccount.com",
	"client_id": "1234567890",
	"auth_uri": "https://accounts.google.com/o/oauth2/auth",
	"token_uri": "https://oauth2.googleapis.com/token",
	"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test-account%40test-project.iam.gserviceaccount.com"
}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gcpCredsSecret,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			gcpCredsSecretKey: []byte(fakeCreds),
		},
	}
}
func TestNewMachineSetActuator(t *testing.T) {
	hiveapi.AddToScheme(scheme.Scheme)
	hivev1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name              string
		clusterDeployment *hivev1.ClusterDeployment
		existingObjects   []runtime.Object
		expectErr         bool
		validate          func(*testing.T)
	}{
		{
			name:              "return an actuator",
			clusterDeployment: testClusterDeployment(),
			existingObjects: []runtime.Object{
				testGCPCredsSecret(),
			},
		},
		{
			name:              "missing secret",
			clusterDeployment: testClusterDeployment(),
			existingObjects:   []runtime.Object{},
			expectErr:         true,
		},
		{
			name:              "secret missing gcp secret key",
			clusterDeployment: testClusterDeployment(),
			existingObjects: []runtime.Object{
				func() *corev1.Secret {
					s := testGCPCredsSecret()
					s.Data = map[string][]byte{
						"badkey": []byte("baddata"),
					}
					return s
				}(),
			},
			expectErr: true,
		},
		{
			name:              "secret with bad gcp secret data",
			clusterDeployment: testClusterDeployment(),
			existingObjects: []runtime.Object{
				func() *corev1.Secret {
					s := testGCPCredsSecret()
					s.Data = map[string][]byte{
						gcpCredsSecretKey: []byte("baddata"),
					}
					return s
				}(),
			},
			expectErr: true,
		},
		{
			name: "unsupported platform",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform = hivev1.Platform{}
				return cd
			}(),
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existingObjects...)
			fakeClient.Create(context.TODO(), test.clusterDeployment)

			logger := log.WithField("actuatorbuilder", "test")
			a, err := newMachineSetActuator(fakeClient, test.clusterDeployment, logger)

			if test.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Implements(t, (*Actuator)(nil), a)
			}
		})
	}
}
