package installmanager

import (
	"encoding/json"

	"github.com/google/uuid"
	installertypes "github.com/openshift/installer/pkg/types"
)

func fakeLoadAdminPassword(m *InstallManager) (string, error) {
	m.log.Warn("loading fake admin password")
	return "fake-password", nil
}

func fakeReadClusterMetadata(m *InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
	m.log.Warn("returning fake cluster metadata")
	clusterID := "fake-cluster-" + uuid.New().String()

	// The caller cares that this contains ClusterID and InfraID.
	// We'll replace this whole thing in a special code path before it's used for deprovision.
	md := &installertypes.ClusterMetadata{
		ClusterID: clusterID,
		InfraID:   clusterID,
	}
	metadataBytes, err := json.Marshal(md)
	if err != nil {
		m.log.WithError(err).Error("error unmarshalling cluster metadata")
		return nil, nil, err
	}

	return metadataBytes, md, nil
}

func fakeProvisionCluster(m *InstallManager) error {
	m.log.Warn("skipping openshift-install create cluster for fake install")
	return nil
}
