package installlogmonitor

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testName      = "mycluster-install-log"
	testCDName    = "mycluster"
	testNamespace = "default"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestIsHiveInstallLogAndMigration(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name                          string
		configMap                     *corev1.ConfigMap
		isHiveInstallLog              bool
		needsMigration                bool
		expectedClusterDeploymentName string
	}{
		{
			name:                          "install log missing labels migration",
			configMap:                     buildInstallLogConfigMapwithoutLabels("mycluster-install-log", "log", "fakelogdata"),
			isHiveInstallLog:              true,
			needsMigration:                true,
			expectedClusterDeploymentName: "mycluster",
		},
		{
			name:             "install log missing labels name mismatch",
			configMap:        buildInstallLogConfigMapwithoutLabels("unrelated-configmap", "log", "fakelogdata"),
			isHiveInstallLog: false,
			needsMigration:   false,
		},
		{
			name:             "install log missing labels key mismatch",
			configMap:        buildInstallLogConfigMapwithoutLabels("mycluster-install-log", "unexpectedkey", "fakelogdata"),
			isHiveInstallLog: false,
			needsMigration:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.configMap)
			r := &ReconcileInstallLog{
				Client: fakeClient,
				scheme: scheme.Scheme,
			}
			isHiveInstallLog, needsMigration := r.isHiveInstallLog(test.configMap)
			assert.Equal(t, test.isHiveInstallLog, isHiveInstallLog)
			assert.Equal(t, test.needsMigration, needsMigration)
			if needsMigration {
				cm := test.configMap.DeepCopy()
				err := r.migrateInstallLog(cm)
				assert.NoError(t, err)
				assert.Equal(t, test.expectedClusterDeploymentName, cm.Labels[hivev1.HiveClusterDeploymentNameLabel])
				assert.Equal(t, "true", cm.Labels[hivev1.HiveInstallLogLabel])
			}
		})
	}

}

const (
	dnsAlreadyExistsLog    = "blahblah\naws_route53_record.api_external: [ERR]: Error building changeset: InvalidChangeBatch: [Tried to create resource record set [name='api.jh-stg-2405-2.n6b3.s1.devshift.org.'type='A'] but it already exists]\n\nblahblah"
	pendingVerificationLog = "blahblah\naws_instance.master.2: Error launching source instance: PendingVerification: Your request for accessing resources in this region is being validated, and you will not be able to launch additional resources in this region until the validation is complete. We will notify you by email once your request has been validated. While normally resolved within minutes, please allow up to 4 hours for this process to complete. If the issue still persists, please let us know by writing to awsa\n\nblahblah"
)

func TestInstallLogProcessing(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name                    string
		existing                []runtime.Object // NOTE: configmap will implicitly be added
		expectedConditionStatus corev1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "process new install log error DNS already exists",
			existing: []runtime.Object{
				buildRegexConfigMap(),
				buildInstallLogConfigMap(testName, "log", dnsAlreadyExistsLog, "false"),
				testClusterDeployment(),
			},
			expectedConditionStatus: corev1.ConditionTrue,
			expectedConditionReason: "DNSAlreadyExists",
		},
		{
			name: "process new install log error PendingVerification",
			existing: []runtime.Object{
				buildRegexConfigMap(),
				buildInstallLogConfigMap(testName, "log", pendingVerificationLog, "false"),
				testClusterDeployment(),
			},
			expectedConditionStatus: corev1.ConditionTrue,
			expectedConditionReason: "PendingVerification",
		},
		{
			name: "process new install log success",
			existing: []runtime.Object{
				buildRegexConfigMap(),
				// Leaving an error in the log so we can test that we do not check for errors if success reported.
				buildInstallLogConfigMap(testName, "log", dnsAlreadyExistsLog, "true"),
				testClusterDeployment(),
			},
		},
		{
			name: "process retry install log success",
			existing: []runtime.Object{
				buildRegexConfigMap(),
				// Leaving an error in the log so we can test that we do not check for errors if success reported.
				buildInstallLogConfigMap(testName, "log", dnsAlreadyExistsLog, "true"),
				testClusterDeploymentWithInstallFailingCondition(corev1.ConditionTrue, "blah", "blah"),
			},
			expectedConditionStatus: corev1.ConditionFalse,
			expectedConditionReason: successReason,
		},
	}

	for _, test := range tests {
		getCD := func(c client.Client) *hivev1.ClusterDeployment {
			cd := &hivev1.ClusterDeployment{}
			err := c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, cd)
			if err == nil {
				return cd
			}
			return nil
		}
		getInstallLog := func(c client.Client) *corev1.ConfigMap {
			cm := &corev1.ConfigMap{}
			err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cm)
			if err == nil {
				return cm
			}
			return nil
		}
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			r := &ReconcileInstallLog{
				Client: fakeClient,
				scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if !assert.NoError(t, err) {
				return
			}

			cd := getCD(fakeClient)
			assert.NotNil(t, cd)

			installLog := getInstallLog(fakeClient)
			processed, ok := installLog.Annotations[processedAnnotation]
			if assert.True(t, ok) {
				assert.Equal(t, "true", processed)
			}

			cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.InstallFailingCondition)
			if test.expectedConditionReason != "" {
				if assert.NotNil(t, cond, "cluster missing InstallFailing condition") {
					assert.Equal(t, test.expectedConditionReason, cond.Reason)
				}
			} else {
				assert.Nil(t, cond, "cluster has InstallFailing condition")
			}

		})
	}

}

// buildInstallLogConfigMap builds an install log configmap.
func buildInstallLogConfigMap(name, key, contents, successStr string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				hivev1.HiveInstallLogLabel:            "true",
				hivev1.HiveClusterDeploymentNameLabel: testCDName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ClusterDeployment",
					Name: testCDName,
				},
			},
		},
		Data: map[string]string{
			key: contents,
		},
	}
	if successStr != "" {
		cm.Data["success"] = successStr
	}

	return cm
}

func buildRegexConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      regexConfigMapName,
			Namespace: hiveNamespace,
		},
		Data: map[string]string{
			"DNSAlreadyExists": `
searchRegexStrings:
- "aws_route53_record.*Error building changeset:.*Tried to create resource record set.*but it already exists"
installFailingReason: DNSAlreadyExists
installFailingMessage: DNS record already exists
`,
			"PendingVerification": `
searchRegexStrings:
- "PendingVerification: Your request for accessing resources in this region is being validated"
installFailingReason: PendingVerification
installFailingMessage: Account pending verification for region
`,
		},
	}
	return cm
}

func buildInstallLogConfigMapwithoutLabels(name, key, contents string) *corev1.ConfigMap {
	cm := buildInstallLogConfigMap(name, key, contents, "")
	cm.ObjectMeta.Labels = map[string]string{}
	return cm
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testCDName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName:  testCDName,
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
		},
		Status: hivev1.ClusterDeploymentStatus{
			ClusterID: "fakeid",
			InfraID:   "fakeid",
			AdminKubeconfigSecret: corev1.LocalObjectReference{
				Name: "kubeconfig-secret",
			},
			Conditions: []hivev1.ClusterDeploymentCondition{},
		},
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return cd
}

func testClusterDeploymentWithInstallFailingCondition(status corev1.ConditionStatus, reason, message string) *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.InstallFailingCondition, corev1.ConditionTrue,
		reason, message, controllerutils.UpdateConditionAlways)
	return cd
}
