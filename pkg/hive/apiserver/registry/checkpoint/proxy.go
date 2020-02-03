package checkpoint

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
		logger:         log.WithField("resource", "checkpoints"),
	})
}

func (s *REST) New() runtime.Object {
	return &hiveapi.Checkpoint{}
}

func (s *REST) NewList() runtime.Object {
	return &hiveapi.CheckpointList{}
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

	checkpoints, err := client.List(optv1)
	if err != nil {
		return nil, err
	}

	ret := &hiveapi.CheckpointList{
		ListMeta: checkpoints.ListMeta,
		Items:    make([]hiveapi.Checkpoint, len(checkpoints.Items)),
	}
	for i, curr := range checkpoints.Items {
		if err := util.CheckpointFromHiveV1(&curr, &ret.Items[i]); err != nil {
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

	checkpoint := &hiveapi.Checkpoint{}
	if err := util.CheckpointFromHiveV1(ret, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
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
	s.logger.WithField("name", obj.(*hiveapi.Checkpoint).Name).Info("create")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	convertedObj := &hivev1.Checkpoint{}
	if err := util.CheckpointToHiveV1(obj.(*hiveapi.Checkpoint), convertedObj); err != nil {
		return nil, err
	}

	ret, err := client.Create(convertedObj)
	if err != nil {
		return nil, err
	}

	checkpoint := &hiveapi.Checkpoint{}
	if err := util.CheckpointFromHiveV1(ret, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	logger := s.logger.WithField("name", name)
	logger.Info("update")

	client, err := s.getClient(ctx)
	if err != nil {
		return nil, false, err
	}

	checkpoint, err := client.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	origCheckpoint := checkpoint.DeepCopy()
	origStatus := origCheckpoint.Status
	origCheckpoint.Status = hivev1.CheckpointStatus{}

	old := &hiveapi.Checkpoint{}
	if err := util.CheckpointFromHiveV1(checkpoint, old); err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, old)
	if err != nil {
		return nil, false, err
	}

	if err := util.CheckpointToHiveV1(obj.(*hiveapi.Checkpoint), checkpoint); err != nil {
		return nil, false, err
	}

	newStatus := checkpoint.Status
	checkpoint.Status = hivev1.CheckpointStatus{}

	if !reflect.DeepEqual(checkpoint, origCheckpoint) {
		logger.Info("forwarding regular update")
		var err error
		checkpoint, err = client.Update(checkpoint)
		if err != nil {
			return nil, false, err
		}
	}

	checkpoint.Status = newStatus
	if !reflect.DeepEqual(newStatus, origStatus) {
		logger.Info("forwarding status update")
		var err error
		checkpoint, err = client.UpdateStatus(checkpoint)
		if err != nil {
			return nil, false, err
		}
	}

	new := &hiveapi.Checkpoint{}
	if err := util.CheckpointFromHiveV1(checkpoint, new); err != nil {
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
	v1Checkpoint, ok := v1Event.Object.(*hivev1.Checkpoint)
	if !ok {
		return nil, errors.Errorf("event object is not a Checkpoint: %T", v1Event.Object)
	}
	v1alpha1Checkpoint := &hiveapi.Checkpoint{}
	if err := util.CheckpointFromHiveV1(v1Checkpoint, v1alpha1Checkpoint); err != nil {
		return nil, errors.Wrap(err, "could not convert checkpoint from v1")
	}
	return &watch.Event{
		Type:   v1Event.Type,
		Object: v1alpha1Checkpoint,
	}, nil
}

func (s *REST) getClient(ctx context.Context) (hivev1client.CheckpointInterface, error) {
	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("namespace parameter required")
	}
	return s.client.Checkpoints(namespace), nil
}
