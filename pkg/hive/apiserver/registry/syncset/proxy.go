package syncset

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
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
	logger log.FieldLogger
}

var _ rest.Lister = &REST{}
var _ rest.Getter = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Watcher = &REST{}

func NewREST(client hivev1client.HiveV1Interface) registry.Storage {
	return registry.WrapStorageError(&REST{
		client:         client,
		TableConvertor: printerstorage.TableConvertor{TablePrinter: printers.NewTablePrinter().With(printersinternal.AddHandlers)},
		logger:         log.WithField("resource", "syncsets"),
	})
}

func (s *REST) New() runtime.Object {
	return &hiveapi.SyncSet{}
}

func (s *REST) NewList() runtime.Object {
	return &hiveapi.SyncSetList{}
}

func (s *REST) NamespaceScoped() bool {
	return true
}

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	s.logger.Info("list")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	syncSets, err := client.List(optv1)
	if err != nil {
		return nil, err
	}

	ret := &hiveapi.SyncSetList{
		ListMeta: syncSets.ListMeta,
		Items:    make([]hiveapi.SyncSet, len(syncSets.Items)),
	}
	for i, curr := range syncSets.Items {
		if err := util.SyncSetFromHiveV1(&curr, &ret.Items[i]); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	s.logger.WithField("name", name).Info("get")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := client.Get(name, *options)
	if err != nil {
		return nil, err
	}

	syncSet := &hiveapi.SyncSet{}
	if err := util.SyncSetFromHiveV1(ret, syncSet); err != nil {
		return nil, err
	}
	return syncSet, nil
}

func (s *REST) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	s.logger.WithField("name", name).Info("delete")

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
	s.logger.WithField("name", obj.(*hiveapi.SyncSet).Name).Info("create")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	convertedObj := &hivev1.SyncSet{}
	if err := util.SyncSetToHiveV1(obj.(*hiveapi.SyncSet), convertedObj); err != nil {
		return nil, err
	}

	ret, err := client.Create(convertedObj)
	if err != nil {
		return nil, err
	}

	syncSet := &hiveapi.SyncSet{}
	if err := util.SyncSetFromHiveV1(ret, syncSet); err != nil {
		return nil, err
	}
	return syncSet, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	logger := s.logger.WithField("name", name)
	logger.Info("update")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, false, err
	}

	syncSet, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	origSyncSet := syncSet.DeepCopy()
	origStatus := origSyncSet.Status
	origSyncSet.Status = hivev1.SyncSetStatus{}

	old := &hiveapi.SyncSet{}
	if err := util.SyncSetFromHiveV1(syncSet, old); err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, old)
	if err != nil {
		return nil, false, err
	}

	if err := util.SyncSetToHiveV1(obj.(*hiveapi.SyncSet), syncSet); err != nil {
		return nil, false, err
	}

	newStatus := syncSet.Status
	syncSet.Status = hivev1.SyncSetStatus{}

	if !reflect.DeepEqual(syncSet, origSyncSet) {
		logger.Info("forwarding regular update")
		var err error
		syncSet, err = client.Update(syncSet)
		if err != nil {
			return nil, false, err
		}
	}

	syncSet.Status = newStatus
	if !reflect.DeepEqual(newStatus, origStatus) {
		logger.Info("forwarding status update")
		var err error
		syncSet, err = client.UpdateStatus(syncSet)
		if err != nil {
			return nil, false, err
		}
	}

	new := &hiveapi.SyncSet{}
	if err := util.SyncSetFromHiveV1(syncSet, new); err != nil {
		return nil, false, err
	}
	return new, false, err
}

func (s *REST) Watch(ctx context.Context, options *metainternal.ListOptions) (watch.Interface, error) {
	s.logger.Info("watch")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	v1watcher, err := client.Watch(optv1)
	if err != nil {
		return nil, err
	}

	return registry.NewProxyWatcher(v1watcher, &eventConverter{}, s.logger), nil
}

type eventConverter struct{}

func (c *eventConverter) Convert(v1Event watch.Event) (*watch.Event, error) {
	v1SyncSet, ok := v1Event.Object.(*hivev1.SyncSet)
	if !ok {
		return nil, errors.Errorf("event object is not a SyncSet: %T", v1Event.Object)
	}
	v1alpha1SyncSet := &hiveapi.SyncSet{}
	if err := util.SyncSetFromHiveV1(v1SyncSet, v1alpha1SyncSet); err != nil {
		return nil, errors.Wrap(err, "could not convert syncSet from v1")
	}
	return &watch.Event{
		Type:   v1Event.Type,
		Object: v1alpha1SyncSet,
	}, nil
}

func (s *REST) getClient(ctx context.Context) (hivev1client.SyncSetInterface, error) {
	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("namespace parameter required")
	}
	return s.client.SyncSets(namespace), nil
}
