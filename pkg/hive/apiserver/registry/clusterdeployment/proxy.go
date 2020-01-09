package clusterdeployment

import (
	"bytes"
	"context"
	"fmt"

	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	installtypes "github.com/openshift/installer/pkg/types"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	"github.com/openshift/hive/pkg/hive/apiserver/registry"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/util"
	printersinternal "github.com/openshift/hive/pkg/printers/internalversion"
)

type REST struct {
	hiveClient hivev1client.HiveV1Interface
	coreClient corev1client.CoreV1Interface
	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.Getter = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Scoper = &REST{}

func NewREST(hiveClient hivev1client.HiveV1Interface, coreClient corev1client.CoreV1Interface) registry.NoWatchStorage {
	return registry.WrapNoWatchStorageError(&REST{
		hiveClient:     hiveClient,
		coreClient:     coreClient,
		TableConvertor: printerstorage.TableConvertor{TablePrinter: printers.NewTablePrinter().With(printersinternal.AddHandlers)},
	})
}

func (s *REST) New() runtime.Object {
	return &hiveapi.ClusterDeployment{}
}

func (s *REST) NewList() runtime.Object {
	return &hiveapi.ClusterDeploymentList{}
}

func (s *REST) NamespaceScoped() bool {
	return true
}

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	cdClient, secretClient, err := s.getClients(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	clusterDeployments, err := cdClient.List(optv1)
	if err != nil {
		return nil, err
	}

	ret := &hiveapi.ClusterDeploymentList{ListMeta: clusterDeployments.ListMeta}
	for _, curr := range clusterDeployments.Items {
		installConfig, _ := getInstallConfig(&curr, secretClient)
		clusterDeployment, err := util.ClusterDeploymentFromHiveV1(&curr, installConfig)
		if err != nil {
			return nil, err
		}
		ret.Items = append(ret.Items, *clusterDeployment)
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	cdClient, secretClient, err := s.getClients(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := cdClient.Get(name, *options)
	if err != nil {
		return nil, err
	}

	installConfig, _ := getInstallConfig(ret, secretClient)

	clusterDeployment, err := util.ClusterDeploymentFromHiveV1(ret, installConfig)
	if err != nil {
		return nil, err
	}
	return clusterDeployment, nil
}

func (s *REST) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	cdClient, _, err := s.getClients(ctx)
	if err != nil {
		return nil, false, err
	}

	if err := cdClient.Delete(name, options); err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, true, nil
}

func (s *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (result runtime.Object, err error) {
	cdClient, secretClient, err := s.getClients(ctx)
	if err != nil {
		return nil, err
	}

	orig := obj.(*hiveapi.ClusterDeployment)

	sshKey := getSSHKey(orig, secretClient)

	convertedObj, installConfig, err := util.ClusterDeploymentToHiveV1(orig, sshKey)
	if err != nil {
		return nil, err
	}

	installConfigData, err := yaml.Marshal(installConfig)
	if err != nil {
		return nil, err
	}

	installConfigSecret, err := createInstallConfigSecret(installConfigData, orig.Name, secretClient)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		secretClient.Delete(installConfigSecret.Name, &metav1.DeleteOptions{})
	}()

	convertedObj.Spec.Provisioning.InstallConfigSecretRef.Name = installConfigSecret.Name

	ret, err := cdClient.Create(convertedObj)
	if err != nil {
		return nil, err
	}

	installConfigSecret.OwnerReferences = append(installConfigSecret.OwnerReferences,
		metav1.OwnerReference{
			APIVersion:         hivev1.SchemeGroupVersion.String(),
			Kind:               "ClusterDeployment",
			Name:               ret.Name,
			UID:                ret.UID,
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	)
	if _, err := secretClient.Update(installConfigSecret); err != nil {
		return nil, err
	}

	clusterDeployment, err := util.ClusterDeploymentFromHiveV1(ret, installConfig)
	if err != nil {
		return nil, err
	}
	return clusterDeployment, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	cdClient, secretClient, err := s.getClients(ctx)
	if err != nil {
		return nil, false, err
	}

	old, err := cdClient.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	oldInstallConfig, oldInstallConfigData := getInstallConfig(old, secretClient)

	oldClusterDeployment, err := util.ClusterDeploymentFromHiveV1(old, oldInstallConfig)
	if err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, oldClusterDeployment)
	if err != nil {
		return nil, false, err
	}

	newClusterDeployment := obj.(*hiveapi.ClusterDeployment)

	sshKey := getSSHKey(newClusterDeployment, secretClient)

	updatedClusterDeployment, updatedInstallConfig, err := util.ClusterDeploymentToHiveV1(newClusterDeployment, sshKey)
	if err != nil {
		return nil, false, err
	}

	updatedInstallConfigData, err := yaml.Marshal(updatedInstallConfig)
	if err != nil {
		return nil, false, err
	}

	if !bytes.Equal(oldInstallConfigData, updatedInstallConfigData) {
		if err := updateInstallConfigSecret(updatedInstallConfigData, old, secretClient); err != nil {
			return nil, false, err
		}
	}

	ret, err := cdClient.Update(updatedClusterDeployment)
	if err != nil {
		return nil, false, err
	}

	clusterDeployment, err := util.ClusterDeploymentFromHiveV1(ret, updatedInstallConfig)
	if err != nil {
		return nil, false, err
	}
	return clusterDeployment, false, err
}

func (s *REST) getClients(ctx context.Context) (hivev1client.ClusterDeploymentInterface, corev1client.SecretInterface, error) {
	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("namespace parameter required")
	}
	return s.hiveClient.ClusterDeployments(namespace),
		s.coreClient.Secrets(namespace),
		nil
}

func getInstallConfig(cd *hivev1.ClusterDeployment, secretClient corev1client.SecretInterface) (*installtypes.InstallConfig, []byte) {
	if cd.Spec.Provisioning == nil {
		return nil, nil
	}
	installConfigSecret, err := secretClient.Get(cd.Spec.Provisioning.InstallConfigSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil
	}
	installConfigData, ok := installConfigSecret.Data["install-config.yaml"]
	if !ok {
		return nil, nil
	}
	installConfig := &installtypes.InstallConfig{}
	if err := yaml.Unmarshal(installConfigData, &installConfig); err != nil {
		return nil, nil
	}
	return installConfig, installConfigData
}

func createInstallConfigSecret(installConfig []byte, cdName string, secretClient corev1client.SecretInterface) (*corev1.Secret, error) {
	return secretClient.Create(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-ic-", cdName),
			},
			Data: map[string][]byte{
				"install-config.yaml": installConfig,
			},
			Type: corev1.SecretTypeOpaque,
		},
	)
}

func updateInstallConfigSecret(installConfig []byte, cd *hivev1.ClusterDeployment, secretClient corev1client.SecretInterface) error {
	installConfigSecret, err := secretClient.Get(cd.Spec.Provisioning.InstallConfigSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	installConfigSecret.Data["install-config.yaml"] = installConfig
	_, err = secretClient.Update(installConfigSecret)
	return err
}

func getSSHKey(cd *hiveapi.ClusterDeployment, secretClient corev1client.SecretInterface) string {
	sshKeySecret, err := secretClient.Get(cd.Spec.SSHKey.Name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return string(sshKeySecret.Data["ssh-publickey"])
}
