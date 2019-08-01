package controller

import (
	"os"
	"strings"

	hiveconstants "github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/velerobackup"
)

func init() {
	if strings.EqualFold(os.Getenv(hiveconstants.VeleroBackupEnvVar), "true") {
		// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
		AddToManagerFuncs = append(AddToManagerFuncs, velerobackup.Add)
	}
}
