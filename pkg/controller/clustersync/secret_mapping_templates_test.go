package clustersync

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testselectorsyncset "github.com/openshift/hive/pkg/test/selectorsyncset"
	teststatefulset "github.com/openshift/hive/pkg/test/statefulset"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestReconcileClusterSync_SecretMappingSourceTemplate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Build(testcd.WithLabel("example.com/cell", "cell-a"))
	syncSet := testselectorsyncset.FullBuilder("writer-credentials", scheme).Build(
		testselectorsyncset.WithGeneration(1),
		testselectorsyncset.WithSecrets(hivev1.SecretMapping{
			SourceRef: hivev1.SecretReference{
				Name:      `writer-{{ fromCDLabel "example.com/cell" }}`,
				Namespace: "credentials",
			},
			TargetRef: hivev1.SecretReference{Name: "writer", Namespace: "monitoring"},
		}),
	)
	syncSet.Spec.EnableResourceTemplates = true
	srcSecret := testsecret.FullBuilder("credentials", "writer-cell-a", scheme).Build(
		testsecret.WithDataKeyValue("tls.crt", []byte("test-certificate")),
	)
	existing := []runtime.Object{
		cd, clusterSyncBuilder(scheme).Build(), syncSet, srcSecret,
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3), teststatefulset.WithReplicas(3)),
	}
	rt := newReconcileTest(mockCtrl, existing...)
	targetSecret := testsecret.FullBuilder("monitoring", "writer", scheme).Build(
		testsecret.WithDataKeyValue("tls.crt", []byte("test-certificate")),
	)
	targetSecret.ResourceVersion = ""
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(targetSecret)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSelectorSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("writer-credentials")}
	rt.run(t)
}

func TestApplySyncSets_SecretMappingSourceTemplates(t *testing.T) {
	const sourceTemplate = `{{ with fromCDLabel "example.com/cell" }}writer-{{ . }}{{ end }}`
	cases := []struct {
		name       string
		sourceName string
		cell       string
		disabled   bool
		wantError  string
		requeue    bool
	}{
		{name: "cell a", sourceName: sourceTemplate, cell: "cell-a"},
		{name: "cell b", sourceName: sourceTemplate, cell: "cell-b"},
		{name: "literal name", sourceName: "writer-cell-a"},
		{name: "literal name with templates disabled", sourceName: "writer-cell-a", disabled: true},
		{
			name: "templates disabled", sourceName: sourceTemplate, cell: "cell-a", disabled: true,
			wantError: "failed to read secret 0: secrets", requeue: true,
		},
		{name: "missing label", sourceName: sourceTemplate, wantError: "invalid source name for secret 0"},
		{name: "invalid rendered name", sourceName: `writer-{{ fromCDLabel "example.com/cell" }}`, cell: "INVALID", wantError: "invalid source name for secret 0"},
		{name: "parse error", sourceName: "{{", wantError: "failed to parse template string"},
		{name: "execution error", sourceName: `{{ index (fromCDLabel "example.com/cell") 100 }}`, cell: "cell-a", wantError: "failed to execute template"},
		{name: "source missing", sourceName: sourceTemplate, cell: "cell-c", wantError: `secrets "writer-cell-c" not found`, requeue: true},
	}
	for _, kind := range []string{"SyncSet", "SelectorSyncSet"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				scheme := scheme.GetScheme()
				cd := cdBuilder(scheme).Build()
				if tc.cell != "" {
					cd.Labels = map[string]string{"example.com/cell": tc.cell}
				}
				mapping := hivev1.SecretMapping{
					SourceRef: hivev1.SecretReference{Name: tc.sourceName, Namespace: "credentials"},
					TargetRef: hivev1.SecretReference{Name: "writer", Namespace: "monitoring"},
				}
				spec := hivev1.SyncSetCommonSpec{
					Secrets:                 []hivev1.SecretMapping{mapping},
					EnableResourceTemplates: !tc.disabled,
					ResourceApplyMode:       hivev1.SyncResourceApplyMode,
				}
				var syncSet CommonSyncSet
				if kind == "SyncSet" {
					syncSet = &SyncSetAsCommon{
						ObjectMeta: metav1.ObjectMeta{Name: "writer-credentials", Namespace: "credentials"},
						Spec:       hivev1.SyncSetSpec{SyncSetCommonSpec: spec},
					}
				} else {
					syncSet = &SelectorSyncSetAsCommon{
						ObjectMeta: metav1.ObjectMeta{Name: "writer-credentials"},
						Spec:       hivev1.SelectorSyncSetSpec{SyncSetCommonSpec: spec},
					}
				}
				var sources []runtime.Object
				for _, cell := range []string{"cell-a", "cell-b"} {
					sources = append(sources, testsecret.FullBuilder("credentials", "writer-"+cell, scheme).Build(
						testsecret.WithType(corev1.SecretTypeTLS),
						testsecret.WithDataKeyValue("tls.crt", []byte("certificate-"+cell)),
						testsecret.WithDataKeyValue("tls.key", []byte("key-"+cell)),
						testsecret.WithDataKeyValue("ca.crt", []byte("ca-"+cell)),
						testsecret.WithDataKeyValue("literal", []byte(sourceTemplate)),
					))
				}
				rt := newReconcileTest(mockCtrl, sources...)
				if tc.wantError == "" {
					index := 0
					if tc.cell == "cell-b" {
						index = 1
					}
					target := sources[index].(*corev1.Secret).DeepCopy()
					target.ObjectMeta = metav1.ObjectMeta{Name: "writer", Namespace: "monitoring"}
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(target)).Return(resource.CreatedApplyResult, nil)
				}
				// Keep the existing destination even if rendering or reading the source fails.
				previous := []hiveintv1alpha1.SyncStatus{buildSyncStatus("writer-credentials",
					withResourcesToDelete(testSecretRef("monitoring", "writer")),
				)}
				statuses, requeue := rt.r.applySyncSets(cd, kind, []CommonSyncSet{syncSet}, previous, true, false, rt.mockResourceHelper, rt.logger)
				require.Len(t, statuses, 1)
				require.Equal(t, tc.requeue, requeue)
				if tc.wantError == "" {
					require.Equal(t, hiveintv1alpha1.SuccessSyncSetResult, statuses[0].Result)
				} else {
					require.Equal(t, hiveintv1alpha1.FailureSyncSetResult, statuses[0].Result)
					require.Contains(t, statuses[0].FailureMessage, tc.wantError)
				}
				require.Equal(t, previous[0].ResourcesToDelete, statuses[0].ResourcesToDelete)
				require.Equal(t, mapping, syncSet.GetSpec().Secrets[0], "the shared mapping must not be mutated")
			})
		}
	}
}
