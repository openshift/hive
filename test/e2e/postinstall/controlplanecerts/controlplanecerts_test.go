package machinesets

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

// TestControlPlaneCerts ensures that our configuration for the APIServer has made it successfully to the spoke cluster
// via a SyncSet.
func TestControlPlaneCerts(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	spokeClient := common.MustGetClientFromConfig(common.MustGetClusterDeploymentClientConfig())
	logger := log.WithField("test", "TestControlPlaneCerts")

	cpss := getControlPlaneCertsSyncSet(cd)
	require.NotNil(t, cpss)
	assert.Equal(t, 0, len(cpss.Spec.Resources), "cp-certs syncset should have no resources")
	assert.Equal(t, 2, len(cpss.Spec.Patches), "unexpected number of patches in cp-certs syncset")
	assert.Equal(t, 1, len(cpss.Spec.Secrets), "unexpected number of secrets in cp-certs syncset")
	targetSecret := cpss.Spec.Secrets[0].TargetRef

	// This should have been applied by the time we make it to this point in the testing.
	apiServerCfg := &configv1.APIServer{}
	err := spokeClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, apiServerCfg)
	require.NoError(t, err)
	logger.Info("fetched APIServer 'cluster' from spoke cluster")
	assert.Equal(t, 1, len(apiServerCfg.Spec.ServingCerts.NamedCertificates))
	assert.Equal(t, 1, len(apiServerCfg.Spec.ServingCerts.NamedCertificates[0].Names))
	assert.Equal(t, fmt.Sprintf("api.%s.%s", cd.Spec.ClusterName, cd.Spec.BaseDomain),
		len(apiServerCfg.Spec.ServingCerts.NamedCertificates[0].Names))
	assert.Equal(t, targetSecret.Name,
		apiServerCfg.Spec.ServingCerts.NamedCertificates[0].ServingCertificate.Name)

	servingCertSecret := &corev1.Secret{}
	err = spokeClient.Get(context.Background(), types.NamespacedName{Name: targetSecret.Name, Namespace: targetSecret.Namespace}, servingCertSecret)
	assert.NoError(t, err)
}

func getControlPlaneCertsSyncSet(cd *hivev1.ClusterDeployment) *hivev1.SyncSet {
	c := common.MustGetClient()
	cpSyncSet := &hivev1.SyncSet{}
	switch err := c.Get(
		context.TODO(),
		types.NamespacedName{Name: fmt.Sprintf("%s-cp-certs", cd.Name), Namespace: cd.Namespace},
		cpSyncSet,
	); {
	case apierrors.IsNotFound(err):
		return nil
	case err != nil:
		log.WithError(err).Fatal("Error fetching syncset")
	}
	return cpSyncSet
}
