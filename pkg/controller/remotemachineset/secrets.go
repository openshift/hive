package remotemachineset

import (
	"fmt"

	"github.com/blang/semver"
)

const (
	// workerUserDataPre46 is the name of a secret in the cluster used for obtaining user data from MCO prior to 4.6.
	workerUserDataPre46 = "worker-user-data"

	// workerUserData46 is the name of a secret in the cluster used for obtaining user data from MCO after 4.6.
	workerUserData46 = "worker-user-data-managed"
)

var (
	workerUserDataAfter46 = semver.MustParseRange(">=4.6.0")
)

func workerUserData(version string) (string, error) {
	parsedVersion, err := semver.ParseTolerant(version)

	if err != nil {
		return "", fmt.Errorf("error parsing version while getting workerUserData secret: %v", err)
	}

	if workerUserDataAfter46(parsedVersion) {
		return workerUserData46, nil
	}

	return workerUserDataPre46, nil
}
