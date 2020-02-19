package labelobjects

import (
	"testing"

	"github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivecontrolplanecerts "github.com/openshift/hive/pkg/controller/controlplanecerts"
	hiveremoteingress "github.com/openshift/hive/pkg/controller/remoteingress"
	hivesyncidentityprovider "github.com/openshift/hive/pkg/controller/syncidentityprovider"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	hiveimageset "github.com/openshift/hive/pkg/imageset"
	"github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/test/job"
	"github.com/openshift/hive/pkg/test/persistentvolumeclaim"
	"github.com/openshift/hive/pkg/test/secret"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	namespace = "notarealns"
)

var (
	emptyRuntimeObjectSlice = []runtime.Object{}
)

func fakeLabelMigrator(existingObjects []runtime.Object) *labelMigrator {
	return &labelMigrator{
		client: fake.NewFakeClient(existingObjects...),
		logger: log.WithField("type", "fake"),
	}
}

func fakeClusterDeployment() *hivev1alpha1.ClusterDeployment {
	return &hivev1alpha1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fakeclusterdeployment",
			Namespace: namespace,
		},
	}
}

func fakeClusterDeprovisionRequest() *hivev1alpha1.ClusterDeprovisionRequest {
	return &hivev1alpha1.ClusterDeprovisionRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fakeclusterdeprovisionrequest",
			Namespace: namespace,
		},
	}
}

func fakeClusterProvision() *hivev1alpha1.ClusterProvision {
	return &hivev1alpha1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fakeclusterprovision",
			Namespace: namespace,
		},
	}
}

func fakeSyncSet() *hivev1alpha1.SyncSet {
	return &hivev1alpha1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fakesyncset",
			Namespace: namespace,
		},
	}
}

func fakeSelectorSyncSet() *hivev1alpha1.SelectorSyncSet {
	return &hivev1alpha1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fakeselectorsyncset",
			Namespace: namespace,
		},
	}
}

func TestRun(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                 string
		existingObjects      []runtime.Object
		expectedError        error
		expectedSuccessCount int
		expectedFailureCount int
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 ClusterProvision",
			existingObjects: []runtime.Object{
				buildClusterProvision(clusterProvisionBase(), genericClusterProvision(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				persistentvolumeclaim.Build(persistentVolumeClaimBase(),
					persistentvolumeclaim.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
					persistentvolumeclaim.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				job.Build(
					jobBase(),
					job.Generic(generic.WithNamePostfix("1")),
					job.Generic(generic.WithLabel(hiveimageset.ImagesetJobLabel, "true")),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildClusterDeprovisionRequest(clusterDeprovisionRequestBase(), genericClusterDeprovisionRequest(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildDNSZone(dnsZoneBase(), genericDNSZone(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				secret.Build(
					secretBase(),
					secret.Generic(generic.WithNamePostfix("1")),
					secret.WithType(corev1.SecretTypeDockerConfigJson),
					secret.WithDataKeyValue(corev1.DockerConfigJsonKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				job.Build(
					jobBase(),
					job.Generic(generic.WithNamePostfix("2")),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeprovisionRequest())),
					job.Generic(generic.WithLabel(constants.UninstallJobLabel, "true")),
				),
				job.Build(
					jobBase(),
					job.Generic(generic.WithNamePostfix("3")),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterProvision())),
					job.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
				),
				buildClusterState(clusterStateBase(), genericClusterState(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivecontrolplanecerts.GenerateControlPlaneCertsSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hiveremoteingress.GenerateRemoteIngressSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivesyncidentityprovider.GenerateIdentityProviderSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				secret.Build(
					secretBase(),
					secret.Generic(generic.WithNamePostfix("2")),
					secret.WithDataKeyValue(constants.KubeconfigSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision()))),
				secret.Build(
					secretBase(),
					secret.Generic(generic.WithNamePostfix("3")),
					secret.WithDataKeyValue(constants.UsernameSecretKey, []byte("NOT REAL")),
					secret.WithDataKeyValue(constants.PasswordSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision()))),
				buildSyncSetInstance(
					syncSetInstanceBase(),
					genericSyncSetInstance(generic.WithNamePostfix("1")),
					withSyncSet(fakeSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
				buildSyncSetInstance(
					syncSetInstanceBase(),
					genericSyncSetInstance(generic.WithNamePostfix("2")),
					withSelectorSyncSet(fakeSelectorSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedSuccessCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			o := Options{
				Logger: log.WithField("type", "fake"),
				DryRun: false,
			}
			fakeClient := fake.NewFakeClient(test.existingObjects...)

			// Act
			lm, actualError := o.run(fakeClient)

			// Assert
			assert.Equal(t, test.expectedError, actualError)

			assert.Equal(t, test.expectedSuccessCount, lm.clusterProvisionsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.installLogPVCsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.imagesetJobsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.clusterDeprovisionsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.dnsZonesSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.mergedPullSecretsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.clusterDeprovisionJobsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.clusterProvisionJobsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.clusterStatesSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.controlPlaneCertificateSyncSetsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.remoteIngressSyncSetsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.identityProviderSyncSetsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.kubeconfigSecretsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.kubeAdminCredsSecretsSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.syncSetInstancesSuccessCount)
			assert.Equal(t, test.expectedSuccessCount, lm.selectorSyncSetInstancesSuccessCount)

			assert.Equal(t, test.expectedFailureCount, lm.clusterProvisionsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.installLogPVCsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.imagesetJobsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.clusterDeprovisionsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.dnsZonesFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.mergedPullSecretsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.clusterDeprovisionJobsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.clusterProvisionJobsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.clusterStatesFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.controlPlaneCertificateSyncSetsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.remoteIngressSyncSetsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.identityProviderSyncSetsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.kubeconfigSecretsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.kubeAdminCredsSecretsFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.syncSetInstancesFailureCount)
			assert.Equal(t, test.expectedFailureCount, lm.selectorSyncSetInstancesFailureCount)
		})
	}
}

func TestLabelClusterProvisions(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 ClusterProvision",
			existingObjects: []runtime.Object{
				buildClusterProvision(clusterProvisionBase(), genericClusterProvision(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildClusterProvision(
					clusterProvisionBase(),
					genericClusterProvision(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericClusterProvision(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelClusterProvisions()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.ClusterProvisionList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelInstallLogPVCs(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 PVC",
			existingObjects: []runtime.Object{
				persistentvolumeclaim.Build(persistentVolumeClaimBase(),
					persistentvolumeclaim.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
					persistentvolumeclaim.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				persistentvolumeclaim.Build(
					persistentVolumeClaimBase(),
					persistentvolumeclaim.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					persistentvolumeclaim.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
					persistentvolumeclaim.Generic(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					persistentvolumeclaim.Generic(generic.WithLabel(constants.PVCTypeLabel, constants.PVCTypeInstallLogs)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelInstallLogPVCs()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&corev1.PersistentVolumeClaimList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelImageSetJobs(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Job",
			existingObjects: []runtime.Object{
				job.Build(jobBase(),
					job.Generic(generic.WithLabel(hiveimageset.ImagesetJobLabel, "true")),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				job.Build(
					jobBase(),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					job.Generic(generic.WithLabel(hiveimageset.ImagesetJobLabel, "true")),
					job.Generic(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					job.Generic(generic.WithLabel(constants.JobTypeLabel, constants.JobTypeImageSet)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelImageSetJobs()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&batchv1.JobList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelClusterDeprovisionRequests(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 ClusterDeprovisionRequest",
			existingObjects: []runtime.Object{
				buildClusterDeprovisionRequest(clusterDeprovisionRequestBase(), genericClusterDeprovisionRequest(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildClusterDeprovisionRequest(
					clusterDeprovisionRequestBase(),
					genericClusterDeprovisionRequest(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericClusterDeprovisionRequest(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelClusterDeprovisionsRequests()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.ClusterDeprovisionRequestList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelDNSZones(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 DNSZone",
			existingObjects: []runtime.Object{
				buildDNSZone(dnsZoneBase(), genericDNSZone(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildDNSZone(
					dnsZoneBase(),
					genericDNSZone(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericDNSZone(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericDNSZone(generic.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelDNSZones()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.DNSZoneList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelMergedPullSecrets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Merged Pull Secret",
			existingObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithType(corev1.SecretTypeDockerConfigJson),
					secret.WithDataKeyValue(corev1.DockerConfigJsonKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithType(corev1.SecretTypeDockerConfigJson),
					secret.WithDataKeyValue(corev1.DockerConfigJsonKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					secret.Generic(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					secret.Generic(generic.WithLabel(constants.SecretTypeLabel, constants.SecretTypeMergedPullSecret)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelMergedPullSecrets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&corev1.SecretList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelClusterDeprovisionJobs(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Job",
			existingObjects: []runtime.Object{
				job.Build(
					jobBase(),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeprovisionRequest())),
					job.Generic(generic.WithLabel(constants.UninstallJobLabel, "true")),
				),
			},
			expectedObjects: []runtime.Object{
				job.Build(
					jobBase(),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterDeprovisionRequest())),
					job.Generic(generic.WithLabel(constants.UninstallJobLabel, "true")),
					job.Generic(generic.WithLabel(constants.ClusterDeprovisionNameLabel, fakeClusterDeprovisionRequest().Name)),
					job.Generic(generic.WithLabel(constants.JobTypeLabel, constants.JobTypeDeprovision)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelClusterDeprovisionJobs()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&batchv1.JobList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelClusterProvisionJobs(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Job",
			existingObjects: []runtime.Object{
				job.Build(
					jobBase(),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterProvision())),
					job.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
				),
			},
			expectedObjects: []runtime.Object{
				job.Build(
					jobBase(),
					job.Generic(generic.WithControllerOwnerReference(fakeClusterProvision())),
					job.Generic(generic.WithLabel(constants.InstallJobLabel, "true")),
					job.Generic(generic.WithLabel(constants.ClusterProvisionNameLabel, fakeClusterProvision().Name)),
					job.Generic(generic.WithLabel(constants.JobTypeLabel, constants.JobTypeProvision)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelClusterProvisionJobs()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&batchv1.JobList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelClusterState(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 ClusterState",
			existingObjects: []runtime.Object{
				buildClusterState(clusterStateBase(), genericClusterState(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildClusterState(
					clusterStateBase(),
					genericClusterState(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericClusterState(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelClusterStates()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.ClusterStateList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelControlPlaneCertificateSyncSets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Control Plane Certificate SyncSet",
			existingObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivecontrolplanecerts.GenerateControlPlaneCertsSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivecontrolplanecerts.GenerateControlPlaneCertsSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericSyncSet(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericSyncSet(generic.WithLabel(constants.SyncSetTypeLabel, constants.SyncSetTypeControlPlaneCerts)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelControlPlaneCertificateSyncSets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.SyncSetList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelRemoteIngressSyncSets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Remote Ingress SyncSet",
			existingObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hiveremoteingress.GenerateRemoteIngressSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hiveremoteingress.GenerateRemoteIngressSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericSyncSet(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericSyncSet(generic.WithLabel(constants.SyncSetTypeLabel, constants.SyncSetTypeRemoteIngress)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelRemoteIngressSyncSets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.SyncSetList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelIdentityProviderSyncSets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Identity Provider SyncSet",
			existingObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivesyncidentityprovider.GenerateIdentityProviderSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildSyncSet(
					syncSetBase(),
					genericSyncSet(generic.WithName(hivesyncidentityprovider.GenerateIdentityProviderSyncSetName(fakeClusterDeployment().Name))),
					genericSyncSet(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericSyncSet(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericSyncSet(generic.WithLabel(constants.SyncSetTypeLabel, constants.SyncSetTypeIdentityProvider)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelIdentityProviderSyncSets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.SyncSetList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelKubeconfigSecrets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Kubeconfig Secret",
			existingObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithDataKeyValue(constants.KubeconfigSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision()))),
			},
			expectedObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithDataKeyValue(constants.KubeconfigSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision())),
					secret.Generic(generic.WithLabel(constants.ClusterProvisionNameLabel, fakeClusterProvision().Name)),
					secret.Generic(generic.WithLabel(constants.SecretTypeLabel, constants.SecretTypeKubeConfig)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelKubeconfigSecrets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&corev1.SecretList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelKubeAdminCredsSecrets(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 Kube Admin Creds Secret",
			existingObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithDataKeyValue(constants.UsernameSecretKey, []byte("NOT REAL")),
					secret.WithDataKeyValue(constants.PasswordSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision()))),
			},
			expectedObjects: []runtime.Object{
				secret.Build(
					secretBase(),
					secret.WithDataKeyValue(constants.UsernameSecretKey, []byte("NOT REAL")),
					secret.WithDataKeyValue(constants.PasswordSecretKey, []byte("NOT REAL")),
					secret.Generic(generic.WithControllerOwnerReference(fakeClusterProvision())),
					secret.Generic(generic.WithLabel(constants.ClusterProvisionNameLabel, fakeClusterProvision().Name)),
					secret.Generic(generic.WithLabel(constants.SecretTypeLabel, constants.SecretTypeKubeAdminCreds)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelKubeAdminCredsSecrets()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&corev1.SecretList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelSyncSetInstances(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 SyncSetInstance",
			existingObjects: []runtime.Object{
				buildSyncSetInstance(
					syncSetInstanceBase(),
					withSyncSet(fakeSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildSyncSetInstance(
					syncSetInstanceBase(),
					withSyncSet(fakeSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericSyncSetInstance(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericSyncSetInstance(generic.WithLabel(constants.SyncSetNameLabel, fakeSyncSet().Name)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelSyncSetInstances()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.SyncSetInstanceList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestLabelSelectorSyncSetInstances(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "Empty Namespace",
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 SelectorSyncSetInstance",
			existingObjects: []runtime.Object{
				buildSyncSetInstance(
					syncSetInstanceBase(),
					withSelectorSyncSet(fakeSelectorSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment()))),
			},
			expectedObjects: []runtime.Object{
				buildSyncSetInstance(
					syncSetInstanceBase(),
					withSelectorSyncSet(fakeSelectorSyncSet()),
					genericSyncSetInstance(generic.WithControllerOwnerReference(fakeClusterDeployment())),
					genericSyncSetInstance(generic.WithLabel(constants.ClusterDeploymentNameLabel, fakeClusterDeployment().Name)),
					genericSyncSetInstance(generic.WithLabel(constants.SelectorSyncSetNameLabel, fakeSelectorSyncSet().Name)),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			l := fakeLabelMigrator(test.existingObjects)

			// Act
			actualError := l.labelSelectorSyncSetInstances()
			actualObjects, _ := controllerutils.GetRuntimeObjects(l.client, []runtime.Object{&hivev1alpha1.SyncSetInstanceList{}}, namespace)

			// Assert
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

type clusterProvisionOption func(*hivev1alpha1.ClusterProvision)

func buildClusterProvision(opts ...clusterProvisionOption) *hivev1alpha1.ClusterProvision {
	retval := &hivev1alpha1.ClusterProvision{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func genericClusterProvision(opt generic.Option) clusterProvisionOption {
	return func(obj *hivev1alpha1.ClusterProvision) {
		opt(obj)
	}
}

type clusterDeprovisionRequestOption func(*hivev1alpha1.ClusterDeprovisionRequest)

func buildClusterDeprovisionRequest(opts ...clusterDeprovisionRequestOption) *hivev1alpha1.ClusterDeprovisionRequest {
	retval := &hivev1alpha1.ClusterDeprovisionRequest{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func genericClusterDeprovisionRequest(opt generic.Option) clusterDeprovisionRequestOption {
	return func(obj *hivev1alpha1.ClusterDeprovisionRequest) {
		opt(obj)
	}
}

type dnsZoneOption func(*hivev1alpha1.DNSZone)

func buildDNSZone(opts ...dnsZoneOption) *hivev1alpha1.DNSZone {
	retval := &hivev1alpha1.DNSZone{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func genericDNSZone(opt generic.Option) dnsZoneOption {
	return func(obj *hivev1alpha1.DNSZone) {
		opt(obj)
	}
}

type clusterStateOption func(*hivev1alpha1.ClusterState)

func buildClusterState(opts ...clusterStateOption) *hivev1alpha1.ClusterState {
	retval := &hivev1alpha1.ClusterState{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func genericClusterState(opt generic.Option) clusterStateOption {
	return func(obj *hivev1alpha1.ClusterState) {
		opt(obj)
	}
}

type syncSetOption func(*hivev1alpha1.SyncSet)

func buildSyncSet(opts ...syncSetOption) *hivev1alpha1.SyncSet {
	retval := &hivev1alpha1.SyncSet{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func genericSyncSet(opt generic.Option) syncSetOption {
	return func(obj *hivev1alpha1.SyncSet) {
		opt(obj)
	}
}

type syncSetInstanceOption func(*hivev1alpha1.SyncSetInstance)

func buildSyncSetInstance(opts ...syncSetInstanceOption) *hivev1alpha1.SyncSetInstance {
	retval := &hivev1alpha1.SyncSetInstance{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

func withSyncSet(syncset *hivev1alpha1.SyncSet) syncSetInstanceOption {
	return func(obj *hivev1alpha1.SyncSetInstance) {
		obj.Spec.SyncSet = &corev1.LocalObjectReference{Name: syncset.Name}
	}
}

func withSelectorSyncSet(selectorsyncset *hivev1alpha1.SelectorSyncSet) syncSetInstanceOption {
	return func(obj *hivev1alpha1.SyncSetInstance) {
		obj.Spec.SelectorSyncSet = &hivev1alpha1.SelectorSyncSetReference{Name: selectorsyncset.Name}
	}
}

func genericSyncSetInstance(opt generic.Option) syncSetInstanceOption {
	return func(obj *hivev1alpha1.SyncSetInstance) {
		opt(obj)
	}
}

func clusterProvisionBase() clusterProvisionOption {
	return func(obj *hivev1alpha1.ClusterProvision) {
		obj.Name = "someclusterprovision"
		obj.Namespace = namespace
	}
}

func clusterDeprovisionRequestBase() clusterDeprovisionRequestOption {
	return func(obj *hivev1alpha1.ClusterDeprovisionRequest) {
		obj.Name = "someclusterdeprovisionrequest"
		obj.Namespace = namespace
	}
}

func persistentVolumeClaimBase() persistentvolumeclaim.Option {
	return func(obj *corev1.PersistentVolumeClaim) {
		obj.Name = "somepvc"
		obj.Namespace = namespace
	}
}

func dnsZoneBase() dnsZoneOption {
	return func(obj *hivev1alpha1.DNSZone) {
		obj.Name = "somednszone"
		obj.Namespace = namespace
	}
}

func jobBase() job.Option {
	return func(obj *batchv1.Job) {
		obj.Name = "somejob"
		obj.Namespace = namespace
	}
}

func secretBase() secret.Option {
	return func(obj *corev1.Secret) {
		obj.Name = "somesecret"
		obj.Namespace = namespace
	}
}

func clusterStateBase() clusterStateOption {
	return func(obj *hivev1alpha1.ClusterState) {
		obj.Name = "someclusterstate"
		obj.Namespace = namespace
	}
}

func syncSetBase() syncSetOption {
	return func(obj *hivev1alpha1.SyncSet) {
		obj.Name = "somesyncset"
		obj.Namespace = namespace
	}
}

func syncSetInstanceBase() syncSetInstanceOption {
	return func(obj *hivev1alpha1.SyncSetInstance) {
		obj.Name = "somesyncsetinstance"
		obj.Namespace = namespace
	}
}
