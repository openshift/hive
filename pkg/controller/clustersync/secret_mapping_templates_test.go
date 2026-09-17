package clustersync

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestApplySyncSet_SecretMappingTemplates(t *testing.T) {
	const sourceTemplate = `{{ fromCDLabel "api.openshift.com/name" }}`
	const clusterName = "hs-mc-o2d6208f0"
	for _, tc := range []struct {
		name, sourceName, wantError string
		enabled, resourceTemplates  bool
		namespaced, missingLabel    bool
	}{
		{name: "SelectorSyncSet", sourceName: sourceTemplate, enabled: true},
		{name: "SyncSet", sourceName: sourceTemplate, enabled: true, namespaced: true},
		{name: "literal", sourceName: clusterName},
		{name: "disabled", sourceName: sourceTemplate, wantError: "not found"},
		{name: "resource templates only", sourceName: sourceTemplate, resourceTemplates: true, wantError: "not found"},
		{name: "missing label", sourceName: sourceTemplate, enabled: true, missingLabel: true, wantError: "invalid source name"},
		{name: "invalid name", sourceName: sourceTemplate + "/invalid", enabled: true, wantError: "invalid source name"},
		{name: "parse error", sourceName: "{{", enabled: true, wantError: "failed to parse template"},
		{name: "execution error", sourceName: `{{ call "invalid" }}`, enabled: true, wantError: "failed to execute template"},
		{name: "missing secret", sourceName: sourceTemplate + "-missing", enabled: true, wantError: "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			cd := cdBuilder(scheme).Build(testcd.WithLabel("api.openshift.com/name", clusterName))
			if tc.missingLabel {
				cd.Labels = nil
			}
			source := testsecret.FullBuilder(testNamespace, clusterName, scheme).Build(
				testsecret.WithDataKeyValue("value", []byte(sourceTemplate)),
			)
			rt := newReconcileTest(gomock.NewController(t), source)
			mapping := hivev1.SecretMapping{
				SourceRef: hivev1.SecretReference{Name: tc.sourceName, Namespace: testNamespace},
				TargetRef: hivev1.SecretReference{Name: "cluster-secret", Namespace: "default"},
			}
			spec := hivev1.SyncSetCommonSpec{
				Secrets:                      []hivev1.SecretMapping{mapping},
				EnableResourceTemplates:      tc.resourceTemplates,
				EnableSecretMappingTemplates: tc.enabled,
			}
			var syncSet CommonSyncSet = &SelectorSyncSetAsCommon{
				Spec: hivev1.SelectorSyncSetSpec{SyncSetCommonSpec: spec},
			}
			if tc.namespaced {
				syncSet = &SyncSetAsCommon{
					ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
					Spec:       hivev1.SyncSetSpec{SyncSetCommonSpec: spec},
				}
			}
			if tc.wantError == "" {
				target := source.DeepCopy()
				target.ObjectMeta = metav1.ObjectMeta{Name: "cluster-secret", Namespace: "default"}
				rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(target)).Return(resource.CreatedApplyResult, nil)
			}
			applied, references, _, err := rt.r.applySyncSet(syncSet, cd, rt.mockResourceHelper, rt.logger)
			if tc.wantError == "" {
				require.NoError(t, err)
				require.Equal(t, references, applied)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			require.Equal(t, []hiveintv1alpha1.SyncResourceReference{testSecretRef("default", "cluster-secret")}, references)
			require.Equal(t, mapping, syncSet.GetSpec().Secrets[0])
		})
	}
}
