package installmanager

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/utils"
)

// NOTE: This is the shape of an AWS metadata.json, but the platform identifier
// is what keys the correct deprovisioner path.
const fakeMetadataFormatStr = `{"%s":{"identifier":[{"kubernetes.io/cluster/fake-infraid":"owned"},{"openshiftClusterID":"%s"}],"region":"us-east-1"},"clusterID":"%s","clusterName":"%s","infraID":"fake-infra-id"}`

func fakeLoadAdminPassword(m *InstallManager) (string, error) {
	m.log.Warn("loading fake admin password")
	return "fake-password", nil
}

func fakeReadClusterMetadata(m *InstallManager, cd *hivev1.ClusterDeployment) ([]byte, *installertypes.ClusterMetadata, error) {
	m.log.Warn("returning fake cluster metadata")
	clusterID := "fake-cluster-" + uuid.New().String()
	metadataBytes := []byte(fmt.Sprintf(fakeMetadataFormatStr, utils.GetClusterPlatform(cd), clusterID, clusterID, m.ClusterProvision.Spec.ClusterDeploymentRef.Name))

	// Extract and save the cluster ID, this step is critical and a failure here
	// should abort the install. Note that this is run *before* we begin provisioning cloud
	// resources.
	md := &installertypes.ClusterMetadata{}
	if err := json.Unmarshal(metadataBytes, md); err != nil {
		m.log.WithError(err).Error("error unmarshalling cluster metadata")
		return nil, nil, err
	}

	return metadataBytes, md, nil
}

func fakeProvisionCluster(m *InstallManager) error {
	m.log.Warn("skipping openshift-install create cluster for fake install")
	return nil
}
