package admission

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

func TestUninstallHive(t *testing.T) {
	c := common.MustGetClient()
	dynClient := common.MustGetDynamicClient()
	logger := log.WithField("test", "TestUninstallHive")

	hiveConfig := &hivev1.HiveConfig{}
	err := c.Get(context.Background(), types.NamespacedName{Name: "hive"}, hiveConfig)
	require.NoError(t, err, "error getting HiveConfig")

	logger.Info("deleting HiveConfig 'hive'")
	err = c.Delete(context.Background(), hiveConfig)
	require.NoError(t, err, "error deleting HiveConfig")

	// Wait for HiveConfig to disappear
	err = common.DynamicWaitForDeletion(dynClient,
		schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "hiveconfigs"},
		"", "hive", logger)
	require.NoError(t, err)

	t.Run("hive-controllers deleted", func(t *testing.T) {
		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			common.GetHiveNamespaceOrDie(), "hive-controllers", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
			common.GetHiveNamespaceOrDie(), "hive-controllers", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"},
			common.GetHiveNamespaceOrDie(), "hive-controllers", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"},
			common.GetHiveNamespaceOrDie(), "hive-frontend", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			"", "hive-controllers", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			"", "hive-frontend", logger)
		require.NoError(t, err)

	})

	t.Run("hiveadmission deleted", func(t *testing.T) {
		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			common.GetHiveNamespaceOrDie(), "hiveadmission", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
			common.GetHiveNamespaceOrDie(), "hiveadmission", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"},
			common.GetHiveNamespaceOrDie(), "hiveadmission", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "system:openshift:hive:hiveadmission"},
			"", "hive-controllers", logger)
		require.NoError(t, err)

		err = common.DynamicWaitForDeletion(dynClient,
			schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"},
			"", "v1.admission.hive.openshift.io", logger)
		require.NoError(t, err)
	})
}
