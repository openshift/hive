package remotemachineset

import (
	"github.com/blang/semver/v4"
)

const (
	// legacyWorkerUserDataName is the name of a secret in the cluster used for obtaining user data from MCO prior to 4.6.
	legacyWorkerUserDataName = "worker-user-data"

	// workerUserDataName is the name of a secret in the cluster used for obtaining user data from MCO after 4.6.
	workerUserDataName = "worker-user-data-managed"
)

var (
	versionsUsingLegacyWorkerUserDataName = semver.MustParseRange("<4.6.0")
)

func workerUserData(version string) string {
	finalizedVersion, err := semver.FinalizeVersion(version)
	if err != nil {
		return workerUserDataName
	}
	parsedVersion, err := semver.ParseTolerant(finalizedVersion)
	if err != nil {
		return workerUserDataName
	}
	if versionsUsingLegacyWorkerUserDataName(parsedVersion) {
		return legacyWorkerUserDataName
	}
	return workerUserDataName
}
