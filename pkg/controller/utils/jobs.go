package utils

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// getJobConditionStatus gets the status of the condition in the job. If the
// condition is not found in the job, then returns False.
func getJobConditionStatus(job *batchv1.Job, conditionType batchv1.JobConditionType) corev1.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionFalse
}

// IsSuccessful returns true if the job was successful
func IsSuccessful(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobComplete) == corev1.ConditionTrue
}

// IsFailed returns true if the job failed
func IsFailed(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobFailed) == corev1.ConditionTrue
}

// IsFinished returns true if the job completed (succeeded or failed)
func IsFinished(job *batchv1.Job) bool {
	return IsSuccessful(job) || IsFailed(job)
}
