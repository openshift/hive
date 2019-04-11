package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
