package metrics

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcs "github.com/openshift/hive/pkg/test/clustersync"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testFinalizer = "test-finalizer"
)

func TestProvisioningUnderwayCollector(t *testing.T) {
	scheme := scheme.GetScheme()

	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).
			GenericOptions(testgeneric.WithCreationTimestamp(time.Now().Add(-2 * time.Hour)))
	}

	cases := []struct {
		name string

		existing []runtime.Object
		min      time.Duration

		expected []string
	}{{
		name: "all installed",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.Installed()),
			cdBuilder("cd-3").Build(testcd.Installed()),
		},
	}, {
		name: "mix of installed and deleting",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Installed()),
			cdBuilder("cd-3").Build(testcd.Installed()),
		},
	}, {
		name: "provisioning with no conditions",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown",
		},
	}, {
		name: "provisioning with other conditions in desired state",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionFalse,
			})),
		},
	}, {
		name: "provisioning with other conditions in undesired state",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionTrue,
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown",
		},
	}, {
		name: "provisioning with Initialized condition",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionUnknown,
				Reason: hivev1.InitializedConditionReason,
			})),
		},
	}, {
		name: "provisioning with ProvisionFailed condition",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas",
		},
	}, {
		name: "provisioning with positive polarity condition",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RequirementsMetCondition,
				Status: corev1.ConditionFalse,
				Reason: "ClusterImageSetNotFound",
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = RequirementsMet image_set = none namespace = cd-2 platform =  reason = ClusterImageSetNotFound",
		},
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
			cdBuilder("cd-3").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas",
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas",
		},
	}, {
		name: "provisioning with no conditions and duration more than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(),
		},
		min: 1 * time.Hour,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown",
		},
	}, {
		name: "provisioning with other conditions and duration more than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionTrue,
			})),
		},
		min: 1 * time.Hour,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown",
		},
	}, {
		name: "provisioning with ProvisionFailed condition and duration more than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		min: 1 * time.Hour,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas",
		},
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition and duration more than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
			cdBuilder("cd-3").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		min: 1 * time.Hour,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas",
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas",
		},
	}, {
		name: "provisioning with no conditions and duration less than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").GenericOptions(testgeneric.WithCreationTimestamp(time.Now().Add(-30 * time.Minute))).Build(),
		},
		min: 1 * time.Hour,
	}, {
		name: "provisioning with other conditions and duration less than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				GenericOptions(testgeneric.WithCreationTimestamp(time.Now().Add(-30 * time.Minute))).
				Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterHibernatingCondition,
					Status: corev1.ConditionTrue,
				})),
		},
		min: 1 * time.Hour,
	}, {
		name: "provisioning with ProvisionFailed condition and duration less than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				GenericOptions(testgeneric.WithCreationTimestamp(time.Now().Add(-30 * time.Minute))).
				Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionFailedCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				})),
		},
		min: 1 * time.Hour,
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition and duration less than min duration",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				GenericOptions(testgeneric.WithCreationTimestamp(time.Now().Add(-30 * time.Minute))).
				Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionFailedCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				})),
			cdBuilder("cd-3").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		min: 1 * time.Hour,
		expected: []string{
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas",
		},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			collect := newProvisioningUnderwaySecondsCollector(c, test.min)
			// TODO: Determine whether collect.Describe() is necessary in test cases
			descCh := make(chan *prometheus.Desc)
			go func() {
				for range descCh {
				}
			}()
			collect.Describe(descCh)
			close(descCh)
			ch := make(chan prometheus.Metric)
			go func() {
				collect.Collect(ch)
				close(ch)
			}()

			var got []string
			for sample := range ch {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got = append(got, metricPretty(&d))
			}
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestProvisioningUnderwayInstallRestartsCollector(t *testing.T) {
	scheme := scheme.GetScheme()

	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme)
	}

	cases := []struct {
		name string

		existing []runtime.Object
		min      int

		expected []string
	}{{
		name: "all installed",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.Installed()),
			cdBuilder("cd-3").Build(testcd.Installed()),
		},
	}, {
		name: "mix of installed and deleting",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Installed()),
			cdBuilder("cd-3").Build(testcd.Installed()),
		},
	}, {
		name: "provisioning with no conditions, zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(),
		},
	}, {
		name: "provisioning with other conditions, zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ClusterHibernatingCondition,
				Status: corev1.ConditionTrue,
			})),
		},
	}, {
		name: "provisioning with ProvisionFailed condition, zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition, zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
			cdBuilder("cd-3").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
	}, {
		name: "provisioning with no conditions, non-zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2)),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown 2",
		},
	}, {
		name: "provisioning with other conditions in desired state",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionFalse,
			})),
		},
	}, {
		name: "provisioning with other conditions in undesired state",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionTrue,
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown 2",
		},
	}, {
		name: "provisioning with ProvisionFailed condition, non-zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas 2",
		},
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition, non-zero restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
			cdBuilder("cd-3").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas 2",
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas 2",
		},
	}, {
		name: "provisioning with no conditions and restarts more than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2)),
		},
		min: 1,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown 2",
		},
	}, {
		name: "provisioning with other conditions and restarts more than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionStoppedCondition,
				Status: corev1.ConditionTrue,
			})),
		},
		min: 1,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = Unknown image_set = none namespace = cd-2 platform =  reason = Unknown 2",
		},
	}, {
		name: "provisioning with ProvisionFailed condition and restarts more than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		min: 1,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas 2",
		},
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition and restarts more than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.ProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
			cdBuilder("cd-3").Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "FailedDueToQuotas",
			})),
		},
		min: 1,
		expected: []string{
			"cluster_deployment = cd-2 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-2 platform =  reason = FailedDueToQuotas 2",
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas 2",
		},
	}, {
		name: "cluster deployment with multiple conditions",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.InstallRestarts(1),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.DNSNotReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: "DNSReady",
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionFailedCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionTrue,
					Reason: "InstallRestartsReached",
				})),
		},
		min: 1,
		expected: []string{
			"cluster_deployment = cd-1 cluster_type = unspecified condition = ProvisionFailed image_set = none namespace = cd-1 platform =  reason = FailedDueToQuotas 1",
		},
	}, {
		name: "provisioning with no conditions and restarts less than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").Build(testcd.InstallRestarts(1)),
		},
		min: 2,
	}, {
		name: "provisioning with other conditions and restarts less than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				Build(testcd.InstallRestarts(1), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterHibernatingCondition,
					Status: corev1.ConditionTrue,
				})),
		},
		min: 2,
	}, {
		name: "provisioning with ProvisionFailed condition and restarts less than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				Build(testcd.InstallRestarts(1), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionFailedCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				})),
		},
		min: 2,
	}, {
		name: "provisioning with ProvisionFailed, DNSNotReadyCondition condition and restarts less than min restarts",
		existing: []runtime.Object{
			cdBuilder("cd-1").Build(testcd.Installed()),
			cdBuilder("cd-2").
				Build(testcd.InstallRestarts(1), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionFailedCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				})),
			cdBuilder("cd-3").
				Build(testcd.InstallRestarts(2), testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.DNSNotReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "FailedDueToQuotas",
				})),
		},
		min: 2,
		expected: []string{
			"cluster_deployment = cd-3 cluster_type = unspecified condition = DNSNotReady image_set = none namespace = cd-3 platform =  reason = FailedDueToQuotas 2",
		},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			collect := newProvisioningUnderwayInstallRestartsCollector(c, test.min)
			// TODO: Determine whether collect.Describe() is necessary in test cases
			descCh := make(chan *prometheus.Desc)
			go func() {
				for range descCh {
				}
			}()
			collect.Describe(descCh)
			close(descCh)
			ch := make(chan prometheus.Metric)
			go func() {
				collect.Collect(ch)
				close(ch)
			}()

			var got []string
			for sample := range ch {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got = append(got, metricPrettyWithValue(&d))
			}
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestDeprovisioningUnderwayCollector(t *testing.T) {
	scheme := scheme.GetScheme()

	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).
			GenericOptions(testgeneric.WithFinalizer(testFinalizer))
	}

	cases := []struct {
		name string

		existing []runtime.Object

		expected1 []string
		expected2 []string
	}{
		{
			name: "all cluster deployment deletion timestamps set",
			existing: []runtime.Object{
				cdBuilder("cd-1").GenericOptions(testgeneric.Deleted()).Build(),
				cdBuilder("cd-2").GenericOptions(testgeneric.Deleted()).Build(),
				cdBuilder("cd-3").GenericOptions(testgeneric.Deleted()).Build(),
			},
			expected1: []string{
				"cluster_deployment = cd-1 cluster_type = unspecified namespace = cd-1",
				"cluster_deployment = cd-2 cluster_type = unspecified namespace = cd-2",
				"cluster_deployment = cd-3 cluster_type = unspecified namespace = cd-3",
			},
			expected2: []string(nil),
		},
		{
			name:      "no cluster deployments",
			existing:  nil,
			expected1: []string(nil),
			expected2: []string(nil),
		},
		{
			name: "some cluster deployment deletion timestamps set",
			existing: []runtime.Object{
				cdBuilder("cd-1").GenericOptions(testgeneric.Deleted()).Build(),
				cdBuilder("cd-2").Build(),
				cdBuilder("cd-3").GenericOptions(testgeneric.Deleted()).Build(),
			},
			expected1: []string{
				"cluster_deployment = cd-1 cluster_type = unspecified namespace = cd-1",
				"cluster_deployment = cd-3 cluster_type = unspecified namespace = cd-3",
			},
			expected2: []string(nil),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			collect := newDeprovisioningUnderwaySecondsCollector(c)
			// TODO: Determine whether collect.Describe() is necessary in test cases
			descCh := make(chan *prometheus.Desc)
			go func() {
				for range descCh {
				}
			}()
			collect.Describe(descCh)
			close(descCh)

			ch1 := make(chan prometheus.Metric)
			go func() {
				collect.Collect(ch1)
				close(ch1)
			}()

			var got1 []string
			for sample := range ch1 {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got1 = append(got1, metricPretty(&d))
			}
			assert.Equal(t, test.expected1, got1)

			cdList := &hivev1.ClusterDeploymentList{}
			require.NoError(t, c.List(context.TODO(), cdList))
			for _, cd := range cdList.Items {
				cd.ObjectMeta.Finalizers = nil
				require.NoError(t, c.Update(context.TODO(), &cd))
			}

			ch2 := make(chan prometheus.Metric)
			go func() {
				collect.Collect(ch2)
				close(ch2)
			}()
			var got2 []string
			for sample := range ch2 {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got2 = append(got2, metricPretty(&d))
			}
			assert.Equal(t, test.expected2, got2)
		})
	}
}

func TestClusterSyncCollector(t *testing.T) {
	scheme := scheme.GetScheme()

	cdBuilder := func(namespace string, name string) testcd.Builder {
		return testcd.FullBuilder(namespace, name, scheme)
	}

	cases := []struct {
		name string

		existing       []runtime.Object
		min            time.Duration
		optionalLabels map[string]string

		expected1 []string
		expected2 []string
	}{
		{
			name: "clustersync did not pass threshold",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
			},
			min:            1 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string(nil),
			expected2:      []string(nil),
		},
		{
			name: "clustersync passed threshold",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
				cdBuilder("test-namespace", "test-name").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.UnreachableCondition,
					Status: corev1.ConditionUnknown,
				})),
			},
			min:            0 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string{"namespaced_name = test-namespace/test-name unreachable = Unknown"},
			expected2:      []string(nil),
		},
		{
			name:           "no clustersync",
			existing:       nil,
			min:            1 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string(nil),
			expected2:      []string(nil),
		},
		{
			name: "report expected fixed labels",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
				cdBuilder("test-namespace", "test-name").Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.UnreachableCondition,
					Status: corev1.ConditionTrue,
				})),
			},
			min:            0 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string{"namespaced_name = test-namespace/test-name unreachable = True"},
			expected2:      []string(nil),
		},
		{
			name: "report  fixed labels; no unreachable condition",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
				cdBuilder("test-namespace", "test-name").Build(),
			},
			min:            0 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string{"namespaced_name = test-namespace/test-name unreachable = unspecified"},
			expected2:      []string(nil),
		},
		{
			name: "report when ClusterDeployment not found",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
			},
			min:            0 * time.Hour,
			optionalLabels: map[string]string{},
			expected1:      []string{"namespaced_name = test-namespace/test-name unreachable = unspecified"},
			expected2:      []string(nil),
		},
		{
			name: "report optional metrics",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
				cdBuilder("test-namespace", "test-name").Build(
					testcd.WithLabel("cd-label", "test-value"),
					testcd.WithCondition(hivev1.ClusterDeploymentCondition{
						Type:   hivev1.UnreachableCondition,
						Status: corev1.ConditionUnknown,
					})),
			},
			min: 0 * time.Hour,
			optionalLabels: map[string]string{
				"test_label": "cd-label",
			},
			expected1: []string{"namespaced_name = test-namespace/test-name test_label = test-value unreachable = Unknown"},
			expected2: []string(nil),
		},
		{
			name: "report multiple optional metrics",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
				cdBuilder("test-namespace", "test-name").Build(
					testcd.WithLabel("cd-label-1", "value-1"),
					testcd.WithLabel("cd-label-2", "value-2"),
					testcd.WithLabel("cd-label-3", "value-3"),
					testcd.WithCondition(hivev1.ClusterDeploymentCondition{
						Type:   hivev1.UnreachableCondition,
						Status: corev1.ConditionUnknown,
					})),
			},
			min: 0 * time.Hour,
			optionalLabels: map[string]string{
				"label1": "cd-label-1",
				"label2": "cd-label-2",
				"label3": "cd-label-3",
			},
			// ensure correct values for all labels are reported
			expected1: []string{"label1 = value-1 label2 = value-2 label3 = value-3 namespaced_name = test-namespace/test-name unreachable = Unknown"},
			expected2: []string(nil),
		},
		{
			name: "report multiple optional metrics; no clusterdeployment found",
			existing: []runtime.Object{
				testcs.FullBuilder("test-namespace", "test-name", scheme).Options(FailingSince(time.Now())).Build(),
			},
			min: 0 * time.Hour,
			optionalLabels: map[string]string{
				"label1": "cd-label-1",
				"label2": "cd-label-2",
				"label3": "cd-label-3",
			},
			// ensure correct values for all labels are reported
			expected1: []string{"label1 = unspecified label2 = unspecified label3 = unspecified namespaced_name = test-namespace/test-name unreachable = unspecified"},
			expected2: []string(nil),
		},
	}
	for _, test := range cases {
		t.Run("test", func(t *testing.T) {
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			collect := newClusterSyncFailingCollector(c, test.min, test.optionalLabels)
			// TODO: Determine whether collect.Describe() is necessary in test cases
			descCh := make(chan *prometheus.Desc)
			go func() {
				for range descCh {
				}
			}()
			collect.Describe(descCh)
			close(descCh)

			ch1 := make(chan prometheus.Metric)
			go func() {
				collect.Collect(ch1)
				close(ch1)
			}()

			var got1 []string
			for sample := range ch1 {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got1 = append(got1, metricPretty(&d))
			}
			assert.Equal(t, test.expected1, got1)

			csList := &hiveintv1alpha1.ClusterSyncList{}
			require.NoError(t, c.List(context.TODO(), csList))
			for _, cs := range csList.Items {
				require.NoError(t, c.Delete(context.TODO(), &cs))
			}

			ch2 := make(chan prometheus.Metric)

			var got2 []string
			go func() {
				collect.Collect(ch2)
				close(ch2)
			}()
			for sample := range ch2 {
				var d dto.Metric
				require.NoError(t, sample.Write(&d))
				got2 = append(got2, metricPretty(&d))
			}
			assert.Equal(t, test.expected2, got2)

		})
	}
}

func FailingSince(t time.Time) testcs.Option {
	return testcs.WithCondition(hiveintv1alpha1.ClusterSyncCondition{
		Type:               hiveintv1alpha1.ClusterSyncFailed,
		Status:             corev1.ConditionTrue,
		Reason:             "foo",
		Message:            "bar",
		LastTransitionTime: metav1.NewTime(t),
	})
}

func metricPretty(d *dto.Metric) string {
	labels := make([]string, 0, len(d.Label))
	for _, label := range d.Label {
		labels = append(labels, fmt.Sprintf("%s = %s", *label.Name, *label.Value))
	}
	return strings.TrimSpace(strings.Join(labels, " "))
}

func metricPrettyWithValue(d *dto.Metric) string {
	labels := metricPretty(d)
	value := 0
	if d.Gauge != nil {
		value = int(*d.Gauge.Value)
	}
	return fmt.Sprintf("%s %d", labels, value)
}
