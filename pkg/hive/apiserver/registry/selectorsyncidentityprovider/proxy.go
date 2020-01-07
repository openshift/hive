package selectorsyncidentityprovider

import (
	"context"

	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

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
	return &hiveapi.SelectorSyncIdentityProvider{}
}

func (s *REST) NewList() runtime.Object {
	return &hiveapi.SelectorSyncIdentityProviderList{}
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

	selectorSyncIdentityProviders, err := client.List(optv1)
	if err != nil {
		return nil, err
	}

	ret := &hiveapi.SelectorSyncIdentityProviderList{ListMeta: selectorSyncIdentityProviders.ListMeta}
	for _, curr := range selectorSyncIdentityProviders.Items {
		selectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderFromHiveV1(&curr)
		if err != nil {
			return nil, err
		}
		ret.Items = append(ret.Items, *selectorSyncIdentityProvider)
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

	selectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderFromHiveV1(ret)
	if err != nil {
		return nil, err
	}
	return selectorSyncIdentityProvider, nil
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

	convertedObj, err := util.SelectorSyncIdentityProviderToHiveV1(obj.(*hiveapi.SelectorSyncIdentityProvider))
	if err != nil {
		return nil, err
	}

	ret, err := client.Create(convertedObj)
	if err != nil {
		return nil, err
	}

	selectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderFromHiveV1(ret)
	if err != nil {
		return nil, err
	}
	return selectorSyncIdentityProvider, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, false, err
	}

	old, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	oldSelectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderFromHiveV1(old)
	if err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, oldSelectorSyncIdentityProvider)
	if err != nil {
		return nil, false, err
	}

	updatedSelectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderToHiveV1(obj.(*hiveapi.SelectorSyncIdentityProvider))
	if err != nil {
		return nil, false, err
	}

	ret, err := client.Update(updatedSelectorSyncIdentityProvider)
	if err != nil {
		return nil, false, err
	}

	selectorSyncIdentityProvider, err := util.SelectorSyncIdentityProviderFromHiveV1(ret)
	if err != nil {
		return nil, false, err
	}
	return selectorSyncIdentityProvider, false, err
}

func (s *REST) getClient(ctx context.Context) (hivev1client.SelectorSyncIdentityProviderInterface, error) {
	return s.client.SelectorSyncIdentityProviders(), nil
}
