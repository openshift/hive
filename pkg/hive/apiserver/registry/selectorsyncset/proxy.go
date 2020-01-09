package selectorsyncset

import (
	"context"

	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	"github.com/openshift/hive/pkg/hive/apiserver/registry"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/util"
	printersinternal "github.com/openshift/hive/pkg/printers/internalversion"
)

type REST struct {
	client hivev1client.HiveV1Interface
	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.Getter = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Scoper = &REST{}

func NewREST(client hivev1client.HiveV1Interface) registry.NoWatchStorage {
	return registry.WrapNoWatchStorageError(&REST{
		client:         client,
		TableConvertor: printerstorage.TableConvertor{TablePrinter: printers.NewTablePrinter().With(printersinternal.AddHandlers)},
	})
}

func (s *REST) New() runtime.Object {
	return &hiveapi.SelectorSyncSet{}
}

func (s *REST) NewList() runtime.Object {
	return &hiveapi.SelectorSyncSetList{}
}

func (s *REST) NamespaceScoped() bool {
	return false
}

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	selectorSyncSets, err := client.List(optv1)
	if err != nil {
		return nil, err
	}

	ret := &hiveapi.SelectorSyncSetList{
		ListMeta: selectorSyncSets.ListMeta,
		Items:    make([]hiveapi.SelectorSyncSet, len(selectorSyncSets.Items)),
	}
	for i, curr := range selectorSyncSets.Items {
		if err := util.SelectorSyncSetFromHiveV1(&curr, &ret.Items[i]); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := client.Get(name, *options)
	if err != nil {
		return nil, err
	}

	selectorSyncSet := &hiveapi.SelectorSyncSet{}
	if err := util.SelectorSyncSetFromHiveV1(ret, selectorSyncSet); err != nil {
		return nil, err
	}
	return selectorSyncSet, nil
}

func (s *REST) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, false, err
	}

	if err := client.Delete(name, options); err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, true, nil
}

func (s *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	convertedObj := &hivev1.SelectorSyncSet{}
	if err := util.SelectorSyncSetToHiveV1(obj.(*hiveapi.SelectorSyncSet), convertedObj); err != nil {
		return nil, err
	}

	ret, err := client.Create(convertedObj)
	if err != nil {
		return nil, err
	}

	selectorSyncSet := &hiveapi.SelectorSyncSet{}
	if err := util.SelectorSyncSetFromHiveV1(ret, selectorSyncSet); err != nil {
		return nil, err
	}
	return selectorSyncSet, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, false, err
	}

	selectorSyncSet, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	old := &hiveapi.SelectorSyncSet{}
	if err := util.SelectorSyncSetFromHiveV1(selectorSyncSet, old); err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, old)
	if err != nil {
		return nil, false, err
	}

	if err := util.SelectorSyncSetToHiveV1(obj.(*hiveapi.SelectorSyncSet), selectorSyncSet); err != nil {
		return nil, false, err
	}

	ret, err := client.Update(selectorSyncSet)
	if err != nil {
		return nil, false, err
	}

	new := &hiveapi.SelectorSyncSet{}
	if err := util.SelectorSyncSetFromHiveV1(ret, new); err != nil {
		return nil, false, err
	}
	return new, false, err
}

func (s *REST) getClient(ctx context.Context) (hivev1client.SelectorSyncSetInterface, error) {
	return s.client.SelectorSyncSets(), nil
}
