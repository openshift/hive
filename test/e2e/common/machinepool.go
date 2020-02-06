package common

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func GetMachinePool(cd *hivev1.ClusterDeployment, poolName string) *hivev1.MachinePool {
	c := MustGetClient()
	pool := &hivev1.MachinePool{}
	switch err := c.Get(
		context.TODO(),
		types.NamespacedName{Name: fmt.Sprintf("%s-%s", cd.Name, poolName), Namespace: cd.Namespace},
		pool,
	); {
	case apierrors.IsNotFound(err):
		return nil
	case err != nil:
		log.WithError(err).Fatal("Error fetching machine pool")
	}
	return pool
}
