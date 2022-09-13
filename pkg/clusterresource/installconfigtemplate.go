package clusterresource

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	installertypes "github.com/openshift/installer/pkg/types"
)

// InstallConfigTemplate allows for overlaying generic InstallConfig with
// parts known to Hive
type InstallConfigTemplate struct {
	MetaData        *metav1.ObjectMeta             `json:"metadata"`
	BaseDomain      string                         `json:"baseDomain"`
	CredentialsMode installertypes.CredentialsMode `json:"credentialsMode,omitempty"`
	raw             map[string]json.RawMessage
}

// UnmarshalJSON will extract the known types in InstallConfigTemplate
func (i *InstallConfigTemplate) UnmarshalJSON(bytes []byte) error {
	if err := json.Unmarshal(bytes, &i.raw); err != nil {
		return err
	}

	if baseDomain, ok := i.raw["baseDomain"]; ok {
		if err := json.Unmarshal(baseDomain, &i.BaseDomain); err != nil {
			return err
		}
	}

	if metadata, ok := i.raw["metadata"]; ok {
		if err := json.Unmarshal(metadata, &i.MetaData); err != nil {
			return err
		}
	}

	if credsMode, ok := i.raw["credentialsMode"]; ok {
		if err := json.Unmarshal(credsMode, &i.CredentialsMode); err != nil {
			return err
		}
	}

	return nil
}

// MarshalJSON will merge the known fields from InstallConfigTemplate
func (i *InstallConfigTemplate) MarshalJSON() ([]byte, error) {
	bd, err := json.Marshal(i.BaseDomain)
	if err != nil {
		return nil, err
	}
	i.raw["baseDomain"] = json.RawMessage(bd)

	md, err := json.Marshal(i.MetaData)
	if err != nil {
		return nil, err
	}
	i.raw["metadata"] = json.RawMessage(md)

	cm, err := json.Marshal(i.CredentialsMode)
	if err != nil {
		return nil, err
	}
	i.raw["credentialsMode"] = json.RawMessage(cm)

	return json.Marshal(i.raw)
}
