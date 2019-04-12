package metrics

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterDeployments(t *testing.T) {
	threeDaysAgo := metav1.Time{Time: time.Now().Add(-72 * time.Hour)}
	tenHoursAgo := metav1.Time{Time: time.Now().Add(-10 * time.Hour)}
	threeHoursAgo := metav1.Time{Time: time.Now().Add(-3 * time.Hour)}
	ninetyMinsAgo := metav1.Time{Time: time.Now().Add(-90 * time.Minute)}
	tenMinsAgo := metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
	fiveMinsAgo := metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
	clusters := []hivev1.ClusterDeployment{
		// Four clusters successfully installed:
		testClusterDeployment("i1", threeDaysAgo, true),
		testClusterDeployment("i2", threeDaysAgo, true),
		testClusterDeployment("i3", threeHoursAgo, true),
		testClusterDeployment("i4", threeHoursAgo, true),

		// Expect three clusters still installing within normal timeframe:
		testClusterDeployment("a", fiveMinsAgo, false),
		testClusterDeployment("b", fiveMinsAgo, false),
		testClusterDeployment("c", tenMinsAgo, false),

		// One cluster installing between 1h and 2h:
		testClusterDeployment("c1", ninetyMinsAgo, false),

		// Expect two clusters still installing over 2h:
		testClusterDeployment("d", threeHoursAgo, false),
		testClusterDeployment("e", threeHoursAgo, false),

		// Expect one cluster also installing over 8h:
		testClusterDeployment("f", tenHoursAgo, false),

		// Expect one cluster also installing over 48h:
		testClusterDeployment("g", threeDaysAgo, false),
	}
	total, installed, uninstalledunder1h, uninstalledover1h, uninstalledover2h, uninstalledover8h, uninstalledover24h := processClusters(clusters, log.WithField("test", "TestClusterDeployments"))
	assert.Equal(t, 12, total)
	assert.Equal(t, 4, installed)
	assert.Equal(t, 3, uninstalledunder1h)
	assert.Equal(t, 5, uninstalledover1h)
	assert.Equal(t, 4, uninstalledover2h)
	assert.Equal(t, 2, uninstalledover8h)
	assert.Equal(t, 1, uninstalledover24h)
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

func testClusterDeployment(name string, created metav1.Time, installed bool) hivev1.ClusterDeployment {
	return hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: created,
		},
		Status: hivev1.ClusterDeploymentStatus{
			Installed: installed,
		},
	}
}
