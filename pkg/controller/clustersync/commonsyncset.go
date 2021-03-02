package clustersync

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// CommonSyncSet is an interface for interacting with SyncSets and SelectorSyncSets in a generic way.
type CommonSyncSet interface {
	// AsRuntimeObject gets the syncset as a runtime.Object
	AsRuntimeObject() runtime.Object

	// AsMetaObject gets the syncset as a metav1.Object
	AsMetaObject() metav1.Object

	// GetSpec gets the common spec of the syncset
	GetSpec() *hivev1.SyncSetCommonSpec
}

// SyncSetAsCommon is a SyncSet typed as a CommonSyncSet
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

// SelectorSyncSetAsCommon is a SelectorSyncSet typed as a CommonSyncSet
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
