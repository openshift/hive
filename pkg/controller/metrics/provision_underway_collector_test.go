package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

func TestProvisioningUnderwayCollector(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)

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
			cdBuilder("cd-2").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
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
			c := fake.NewFakeClientWithScheme(scheme, test.existing...)
			collect := newProvisioningUnderwaySecondsCollector(c, test.min)

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
				got = append(got, metricPretty(d))
			}
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestProvisioningUnderwayInstallRestartsCollector(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)

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
			cdBuilder("cd-2").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
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
			c := fake.NewFakeClientWithScheme(scheme, test.existing...)
			collect := newProvisioningUnderwayInstallRestartsCollector(c, test.min)

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
				got = append(got, metricPrettyWithValue(d))
			}
			assert.Equal(t, test.expected, got)
		})
	}
}
func metricPretty(d dto.Metric) string {
	labels := make([]string, len(d.Label))
	for _, label := range d.Label {
		labels = append(labels, fmt.Sprintf("%s = %s", *label.Name, *label.Value))
	}
	return strings.TrimSpace(strings.Join(labels, " "))
}

func metricPrettyWithValue(d dto.Metric) string {
	labels := metricPretty(d)
	value := 0
	if d.Gauge != nil {
		value = int(*d.Gauge.Value)
	}
	return fmt.Sprintf("%s %d", labels, value)
}
