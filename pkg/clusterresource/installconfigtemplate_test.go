package clusterresource

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fakeInstallConfigTemplateJSON = `{"apiVersion":"v1","baseDomain":"fake.domain","metadata": {"name":"my_own_cluster"},"compute": [{"hyperthreading":"Enabled","name":"worker","platform": {"aws": {"type":"m5.2xlarge","rootVolume": {"size": 128},"zones": ["eu-west-1a","eu-west-1b","eu-west-1c"]}},"replicas": 3}],"controlPlane": {"hyperthreading":"Enabled","name":"master","platform": {"aws": {"type":"m5.xlarge","rootVolume": {"size": 128},"zones": ["eu-west-1a","eu-west-1b","eu-west-1c"]}},"replicas": 3},"networking": {"clusterNetwork": [{"cidr":"10.128.0.0/14","hostPrefix": 23}],"machineCIDR":"10.0.0.0/16","networkType":"OpenShiftSDN","serviceNetwork": ["172.30.0.0/16"]},"platform": {"aws": {"region":"eu-west-1","userTags": {"owner":"myself","team":"ICPMCM","Usage":"Temp","Usage_desc":"Development environment for OpenShift","Review_freq":"week"}}},"pullSecret":"foobar"}`
)

func TestInstallConfigTemplateUnMarshalling(t *testing.T) {

	d := new(InstallConfigTemplate)
	err := json.Unmarshal([]byte(fakeInstallConfigTemplateJSON), d)
	require.NoError(t, err)

	assert.Equal(t, d.BaseDomain, string("fake.domain"))
	assert.Equal(t, d.MetaData.Name, string("my_own_cluster"))
}

func getUpdatedJSONString(input string, updates []string) string {
	re := strings.NewReplacer(updates...)
	return re.Replace(input)
}

func TestInstallConfigTemplate_MarshalJSON(t *testing.T) {
	type fields struct {
		MetaData   *metav1.ObjectMeta
		BaseDomain string
		original   string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{
			name: "Unchanged",
			fields: fields{
				original: fakeInstallConfigTemplateJSON,
			},
			want: fakeInstallConfigTemplateJSON,
		},
		{
			name: "Only update baseDomain",
			fields: fields{
				original:   fakeInstallConfigTemplateJSON,
				BaseDomain: baseDomain,
			},
			want: getUpdatedJSONString(fakeInstallConfigTemplateJSON, []string{"fake.domain", baseDomain}),
		},
		{
			name: "Only update metadata",
			fields: fields{
				original: fakeInstallConfigTemplateJSON,
				MetaData: &metav1.ObjectMeta{
					Name: clusterName,
				},
			},
			want: getUpdatedJSONString(fakeInstallConfigTemplateJSON, []string{"my_own_cluster", clusterName}),
		},
		{
			name: "Update domain and clustername",
			fields: fields{
				original:   fakeInstallConfigTemplateJSON,
				BaseDomain: baseDomain,
				MetaData: &metav1.ObjectMeta{
					Name: clusterName,
				},
			},
			want: getUpdatedJSONString(fakeInstallConfigTemplateJSON, []string{"my_own_cluster", clusterName, "fake.domain", baseDomain}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := new(InstallConfigTemplate)
			err := json.Unmarshal([]byte(tt.fields.original), i)
			require.NoError(t, err)

			if tt.fields.BaseDomain != "" {
				i.BaseDomain = tt.fields.BaseDomain
			}
			if tt.fields.MetaData != nil {
				i.MetaData = tt.fields.MetaData
			}

			got, err := i.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("InstallConfigTemplate.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.JSONEq(t, string(got), tt.want)
		})
	}
}
