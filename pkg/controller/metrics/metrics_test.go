package metrics

import (
	"testing"
	"time"

	//log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterAccumulator(t *testing.T) {
	tenDaysAgo := metav1.Time{Time: time.Now().Add(-10 * 24 * time.Hour)}
	tenHoursAgo := metav1.Time{Time: time.Now().Add(-10 * time.Hour)}
	threeHoursAgo := metav1.Time{Time: time.Now().Add(-3 * time.Hour)}
	ninetyMinsAgo := metav1.Time{Time: time.Now().Add(-90 * time.Minute)}
	tenMinsAgo := metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
	fiveMinsAgo := metav1.Time{Time: time.Now().Add(-5 * time.Minute)}

	clusters := []hivev1.ClusterDeployment{
		// Four managed clusters successfully installed:
		testClusterDeployment("i1", "managed", tenDaysAgo, true),
		testClusterDeployment("i2", "managed", tenDaysAgo, true),
		testClusterDeployment("i3", "managed", threeHoursAgo, true),
		testClusterDeployment("i4", "managed", threeHoursAgo, true),

		// Expect three managed clusters still installing within normal timeframe:
		testClusterDeploymentWithConditions("a", "managed", fiveMinsAgo, false,
			[]hivev1.ClusterDeploymentConditionType{
				hivev1.ClusterImageSetNotFoundCondition,
			}),
		testClusterDeploymentWithConditions("b", "managed", fiveMinsAgo, false,
			[]hivev1.ClusterDeploymentConditionType{
				hivev1.ClusterImageSetNotFoundCondition,
				hivev1.ControlPlaneCertificateNotFoundCondition,
			}),
		testClusterDeployment("c", "managed", tenMinsAgo, false),

		// One managed cluster installing between 1h and 2h:
		testClusterDeployment("c1", "managed", ninetyMinsAgo, false),

		// Expect two managed clusters still installing over 2h:
		testClusterDeploymentWithConditions("d", "managed", threeHoursAgo, false,
			[]hivev1.ClusterDeploymentConditionType{
				hivev1.IngressCertificateNotFoundCondition,
			}),
		testClusterDeployment("e", "managed", threeHoursAgo, false),

		// Expect one managed cluster also installing over 8h:
		testClusterDeployment("f", "managed", tenHoursAgo, false),

		// Expect one managed cluster also installing over 72h:
		testClusterDeployment("g", "managed", tenDaysAgo, false),

		// Expect a few unmanaged clusters to test different types as well:
		testClusterDeployment("unmanaged1", "unmanaged", tenDaysAgo, true),
		testClusterDeployment("unmanaged1", "unmanaged", fiveMinsAgo, false),
		testClusterDeployment("unmanaged1", "unmanaged", threeHoursAgo, false),
		testClusterDeployment("unmanaged1", "unmanaged", tenDaysAgo, false),
	}

	accumulator, _ := newClusterAccumulator(infinity, []string{"0h", "1h", "2h", "8h", "24h", "72h"})
	for _, cd := range clusters {
		accumulator.processCluster(&cd)
	}

	assert.Equal(t, 12, accumulator.total["managed"])
	assert.Equal(t, 4, accumulator.installed["managed"])
	assert.Equal(t, 8, accumulator.uninstalled["0h"]["managed"])
	assert.Equal(t, 5, accumulator.uninstalled["1h"]["managed"])
	assert.Equal(t, 4, accumulator.uninstalled["2h"]["managed"])
	assert.Equal(t, 2, accumulator.uninstalled["8h"]["managed"])
	assert.Equal(t, 1, accumulator.uninstalled["24h"]["managed"])
	assert.Equal(t, 1, accumulator.uninstalled["72h"]["managed"])

	assert.Equal(t, 4, accumulator.total["unmanaged"])
	assert.Equal(t, 1, accumulator.installed["unmanaged"])
	assert.Equal(t, 3, accumulator.uninstalled["0h"]["unmanaged"])
	assert.Equal(t, 2, accumulator.uninstalled["1h"]["unmanaged"])
	assert.Equal(t, 2, accumulator.uninstalled["2h"]["unmanaged"])
	assert.Equal(t, 1, accumulator.uninstalled["8h"]["unmanaged"])
	assert.Equal(t, 1, accumulator.uninstalled["24h"]["unmanaged"])
	assert.Equal(t, 1, accumulator.uninstalled["72h"]["unmanaged"])

	assert.Equal(t, 2, accumulator.conditions[hivev1.ClusterImageSetNotFoundCondition]["managed"])
	assert.Equal(t, 1, accumulator.conditions[hivev1.ControlPlaneCertificateNotFoundCondition]["managed"])
	assert.Equal(t, 1, accumulator.conditions[hivev1.IngressCertificateNotFoundCondition]["managed"])

	// Also test with a cluster age filter:
	accumulator, _ = newClusterAccumulator("8h", []string{"0h", "1h", "2h", "8h", "24h", "72h"})
	for _, cd := range clusters {
		accumulator.processCluster(&cd)
	}
	assert.Equal(t, 8, accumulator.total["managed"])
	assert.Equal(t, 2, accumulator.installed["managed"])
	assert.Equal(t, 6, accumulator.uninstalled["0h"]["managed"])
	assert.Equal(t, 3, accumulator.uninstalled["1h"]["managed"])
	assert.Equal(t, 2, accumulator.uninstalled["2h"]["managed"])
	assert.Equal(t, 0, accumulator.uninstalled["8h"]["managed"])
	assert.Equal(t, 0, accumulator.uninstalled["24h"]["managed"])
	assert.Equal(t, 0, accumulator.uninstalled["72h"]["managed"])
}

func TestInstallJobs(t *testing.T) {
	oneHourAgo := &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
	fiveMinsAgo := &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
	jobs := []batchv1.Job{
		{
			Status: batchv1.JobStatus{
				StartTime:      oneHourAgo,
				CompletionTime: fiveMinsAgo,
			},
		},
		{
			// Job that hasn't finished:
			Status: batchv1.JobStatus{
				StartTime: oneHourAgo,
			},
		},
		{
			// Job that hasn't finished:
			Status: batchv1.JobStatus{
				StartTime: fiveMinsAgo,
			},
		},
		{
			// Job that has failed:
			Status: batchv1.JobStatus{
				StartTime: oneHourAgo,
				Failed:    1,
			},
		},
	}
	running, failed := processJobs(jobs)
	assert.Equal(t, 2, running)
	assert.Equal(t, 1, failed)
}

func testClusterDeployment(name, clusterType string, created metav1.Time, installed bool) hivev1.ClusterDeployment {
	return hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: created,
			Labels:            map[string]string{hivev1.HiveClusterTypeLabel: clusterType},
		},
		Status: hivev1.ClusterDeploymentStatus{
			Installed: installed,
		},
	}
}

func testClusterDeploymentWithConditions(name, clusterType string, created metav1.Time, installed bool,
	conditions []hivev1.ClusterDeploymentConditionType) hivev1.ClusterDeployment {

	cd := testClusterDeployment(name, clusterType, created, installed)
	for _, c := range conditions {
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
			cd.Status.Conditions,
			c,
			corev1.ConditionTrue,
			"NobodyCares",
			"Really.",
			controllerutils.UpdateConditionNever)
	}
	return cd

}
