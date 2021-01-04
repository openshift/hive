package installmanager

import (
	"encoding/json"
	"fmt"

	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

const fakeMetadataFormatStr = `{"aws":{"identifier":[{"kubernetes.io/cluster/fake-infraid":"owned"},{"openshiftClusterID":"fake-cluster-id"}],"region":"us-east-1"},"clusterID":"fake-cluster-id","clusterName":"%s","infraID":"fake-infra-id"}`

func fakeLoadAdminPassword(m *InstallManager) (string, error) {
	m.log.Warn("loading fake admin password")
	return "fake-password", nil
}

func fakeReadClusterMetadata(provision *hivev1.ClusterProvision, m *InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
	m.log.Warn("returning fake cluster metadata")
	metadataBytes := []byte(fmt.Sprintf(fakeMetadataFormatStr, provision.Spec.ClusterDeploymentRef.Name))

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
