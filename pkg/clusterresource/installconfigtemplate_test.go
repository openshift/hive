package clusterresource

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fakeInstallConfigTemplateJSON = `{"apiVersion":"v1","baseDomain":"fake.domain","metadata": {"name":"my_own_cluster","creationTimestamp":null},"compute": [{"hyperthreading":"Enabled","name":"worker","platform": {"aws": {"type":"m5.2xlarge","rootVolume": {"size": 128},"zones": ["eu-west-1a","eu-west-1b","eu-west-1c"]}},"replicas": 3}],"controlPlane": {"hyperthreading":"Enabled","name":"master","platform": {"aws": {"type":"m5.xlarge","rootVolume": {"size": 128},"zones": ["eu-west-1a","eu-west-1b","eu-west-1c"]}},"replicas": 3},"networking": {"clusterNetwork": [{"cidr":"10.128.0.0/14","hostPrefix": 23}],"machineCIDR":"10.0.0.0/16","networkType":"OpenShiftSDN","serviceNetwork": ["172.30.0.0/16"]},"platform": {"aws": {"region":"eu-west-1","userTags": {"owner":"myself","team":"ICPMCM","Usage":"Temp","Usage_desc":"Development environment for OpenShift","Review_freq":"week"}}},"pullSecret":"foobar"}`
)

func TestInstallConfigTemplateUnMarshalling(t *testing.T) {

	d := new(InstallConfigTemplate)
	err := json.Unmarshal([]byte(fakeInstallConfigTemplateJSON), d)
	require.NoError(t, err)

	assert.Equal(t, d.BaseDomain, string("fake.domain"))
	assert.Equal(t, d.MetaData.Name, string("my_own_cluster"))
}

func TestInstallConfigTemplateMarshalling(t *testing.T) {

	{
		d := new(InstallConfigTemplate)
		err := json.Unmarshal([]byte(fakeInstallConfigTemplateJSON), d)
		require.NoError(t, err)

		d.BaseDomain = baseDomain
		d.MetaData.Name = clusterName

		s, err := json.Marshal(d)
		require.NoError(t, err)

		re := strings.NewReplacer("fake.domain", baseDomain, "my_own_cluster", clusterName)
		assert.JSONEq(t, string(s), re.Replace(fakeInstallConfigTemplateJSON))
	}
	{
		d := new(InstallConfigTemplate)
		err := json.Unmarshal([]byte(fakeInstallConfigTemplateJSON), d)
		require.NoError(t, err)

		d.MetaData.Name = clusterName

		s, err := json.Marshal(d)
		require.NoError(t, err)

		re := strings.NewReplacer("my_own_cluster", clusterName)
		assert.JSONEq(t, string(s), re.Replace(fakeInstallConfigTemplateJSON))
	}
	{
		d := new(InstallConfigTemplate)
		err := json.Unmarshal([]byte(fakeInstallConfigTemplateJSON), d)
		require.NoError(t, err)

		d.BaseDomain = baseDomain

		s, err := json.Marshal(d)
		require.NoError(t, err)

		re := strings.NewReplacer("fake.domain", baseDomain)
		assert.JSONEq(t, string(s), re.Replace(fakeInstallConfigTemplateJSON))
	}

}
