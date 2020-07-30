package clustersyncset

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

type CommonSyncSet interface {
	AsRuntimeObject() runtime.Object

	AsMetaObject() metav1.Object

	GetSpec() *hivev1.SyncSetCommonSpec

	GetStatus() *hivev1.SyncSetCommonStatus
}

type SyncSetAsCommon hivev1.SyncSet

func (s *SyncSetAsCommon) AsRuntimeObject() runtime.Object {
	return (*hivev1.SyncSet)(s)
}

func (s *SyncSetAsCommon) AsMetaObject() metav1.Object {
	return (*hivev1.SyncSet)(s)
}

func (s *SyncSetAsCommon) GetSpec() *hivev1.SyncSetCommonSpec {
	return &s.Spec.SyncSetCommonSpec
}

func (s *SyncSetAsCommon) GetStatus() *hivev1.SyncSetCommonStatus {
	return &s.Status.SyncSetCommonStatus
}

type SelectorSyncSetAsCommon hivev1.SelectorSyncSet

func (s *SelectorSyncSetAsCommon) AsRuntimeObject() runtime.Object {
	return (*hivev1.SelectorSyncSet)(s)
}

func (s *SelectorSyncSetAsCommon) AsMetaObject() metav1.Object {
	return (*hivev1.SelectorSyncSet)(s)
}

func (s *SelectorSyncSetAsCommon) GetSpec() *hivev1.SyncSetCommonSpec {
	return &s.Spec.SyncSetCommonSpec
}

func (s *SelectorSyncSetAsCommon) GetStatus() *hivev1.SyncSetCommonStatus {
	return &s.Status.SyncSetCommonStatus
}
