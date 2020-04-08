package hiveapiserver

import (
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/server/options"
	apiserverstorage "k8s.io/apiserver/pkg/server/storage"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
)

// NewRESTOptionsGetter returns a restoptions.Getter implemented using information from the provided master config.
func NewRESTOptionsGetter(etcdOptions *options.EtcdOptions) (genericregistry.RESTOptionsGetter, error) {
	storageFactory := apiserverstorage.NewDefaultStorageFactory(
		etcdOptions.StorageConfig,
		etcdOptions.DefaultStorageMediaType,
		legacyscheme.Codecs,
		apiserverstorage.NewDefaultResourceEncodingConfig(legacyscheme.Scheme),
		&serverstorage.ResourceConfig{},
		nil,
	)

	restOptionsGetter := &options.StorageFactoryRestOptionsFactory{
		Options:        *etcdOptions,
		StorageFactory: storageFactory,
	}
	return restOptionsGetter, nil
}
