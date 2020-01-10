package clusterdeployment

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

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

	"github.com/openshift/library-go/pkg/controller"

	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"

	installtypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	"github.com/openshift/hive/pkg/hive/apiserver/registry"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/util"
	printersinternal "github.com/openshift/hive/pkg/printers/internalversion"
)

const (
	installConfigKey = "install-config.yaml"
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
	cdClient, secretClient, machinePoolClient, err := s.getClients(ctx)
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

	ret := &hiveapi.ClusterDeploymentList{
		ListMeta: clusterDeployments.ListMeta,
		Items:    make([]hiveapi.ClusterDeployment, len(clusterDeployments.Items)),
	}
	for i, curr := range clusterDeployments.Items {
		installConfig, _ := getInstallConfig(&curr, secretClient)
		machinePools, err := getMachinePools(curr.Name, machinePoolClient)
		if err != nil {
			return nil, err
		}
		if err := util.ClusterDeploymentFromHiveV1(&curr, installConfig, machinePools, &ret.Items[i]); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	cdClient, secretClient, machinePoolClient, err := s.getClients(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := cdClient.Get(name, *options)
	if err != nil {
		return nil, err
	}

	installConfig, _ := getInstallConfig(ret, secretClient)
	machinePools, err := getMachinePools(ret.Name, machinePoolClient)
	if err != nil {
		return nil, err
	}

	clusterDeployment := &hiveapi.ClusterDeployment{}
	if err := util.ClusterDeploymentFromHiveV1(ret, installConfig, machinePools, clusterDeployment); err != nil {
		return nil, err
	}
	return clusterDeployment, nil
}

func (s *REST) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	cdClient, _, _, err := s.getClients(ctx)
	if err != nil {
		return nil, false, err
	}

	if err := cdClient.Delete(name, options); err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, true, nil
}

func (s *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (result runtime.Object, err error) {
	cdClient, secretClient, machinePoolClient, err := s.getClients(ctx)
	if err != nil {
		return nil, err
	}

	orig := obj.(*hiveapi.ClusterDeployment)

	sshKey := getSSHKey(orig, secretClient)

	machinePools, err := getMachinePools(orig.Name, machinePoolClient)
	if err != nil {
		return nil, err
	}
	oldMachinePools := make([]*hivev1.MachinePool, len(machinePools))
	for i, p := range machinePools {
		oldMachinePools[i] = p.DeepCopy()
	}

	convertedObj := &hivev1.ClusterDeployment{}
	installConfig := &installtypes.InstallConfig{}
	if err := util.ClusterDeploymentToHiveV1(orig, sshKey, convertedObj, installConfig, &machinePools); err != nil {
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
	defer func() {
		if err == nil {
			return
		}
		cdClient.Delete(convertedObj.Name, &metav1.DeleteOptions{})
	}()

	machinePools, err = reconcileMachinePools(oldMachinePools, machinePools, ret, machinePoolClient)
	if err != nil {
		return nil, err
	}

	clusterDeployment := &hiveapi.ClusterDeployment{}
	if err := util.ClusterDeploymentFromHiveV1(ret, installConfig, machinePools, clusterDeployment); err != nil {
		return nil, err
	}
	return clusterDeployment, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	cdClient, secretClient, machinePoolClient, err := s.getClients(ctx)
	if err != nil {
		return nil, false, err
	}

	clusterDeployment, err := cdClient.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	installConfig, installConfigSecret := getInstallConfig(clusterDeployment, secretClient)
	machinePools, err := getMachinePools(name, machinePoolClient)
	if err != nil {
		return nil, false, err
	}
	oldMachinePools := make([]*hivev1.MachinePool, len(machinePools))
	for i, p := range machinePools {
		oldMachinePools[i] = p.DeepCopy()
	}

	old := &hiveapi.ClusterDeployment{}
	if err := util.ClusterDeploymentFromHiveV1(clusterDeployment, installConfig, machinePools, old); err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, old)
	if err != nil {
		return nil, false, err
	}

	newClusterDeployment := obj.(*hiveapi.ClusterDeployment)

	sshKey := getSSHKey(newClusterDeployment, secretClient)

	if err := util.ClusterDeploymentToHiveV1(newClusterDeployment, sshKey, clusterDeployment, installConfig, &machinePools); err != nil {
		return nil, false, err
	}

	_, err = updateInstallConfigSecret(installConfig, installConfigSecret, clusterDeployment, secretClient)
	if err != nil {
		return nil, false, err
	}

	machinePools, err = reconcileMachinePools(oldMachinePools, machinePools, clusterDeployment, machinePoolClient)
	if err != nil {
		return nil, false, err
	}

	ret, err := cdClient.Update(clusterDeployment)
	if err != nil {
		return nil, false, err
	}

	new := &hiveapi.ClusterDeployment{}
	if err := util.ClusterDeploymentFromHiveV1(ret, installConfig, machinePools, new); err != nil {
		return nil, false, err
	}
	return new, false, err
}

func (s *REST) getClients(ctx context.Context) (hivev1client.ClusterDeploymentInterface, corev1client.SecretInterface, hivev1client.MachinePoolInterface, error) {
	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, nil, nil, apierrors.NewBadRequest("namespace parameter required")
	}
	return s.hiveClient.ClusterDeployments(namespace),
		s.coreClient.Secrets(namespace),
		s.hiveClient.MachinePools(namespace),
		nil
}

func getInstallConfig(cd *hivev1.ClusterDeployment, secretClient corev1client.SecretInterface) (*installtypes.InstallConfig, *corev1.Secret) {
	if cd.Spec.Provisioning == nil {
		return nil, nil
	}
	installConfigSecret, err := secretClient.Get(cd.Spec.Provisioning.InstallConfigSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil
	}
	installConfigData, ok := installConfigSecret.Data[installConfigKey]
	if !ok {
		return nil, nil
	}
	installConfig := &installtypes.InstallConfig{}
	if err := yaml.Unmarshal(installConfigData, &installConfig); err != nil {
		return nil, nil
	}
	return installConfig, installConfigSecret
}

func createInstallConfigSecret(installConfig []byte, cdName string, secretClient corev1client.SecretInterface) (*corev1.Secret, error) {
	return secretClient.Create(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-ic-", cdName),
			},
			Data: map[string][]byte{
				installConfigKey: installConfig,
			},
			Type: corev1.SecretTypeOpaque,
		},
	)
}

func updateInstallConfigSecret(installConfig *installtypes.InstallConfig, installConfigSecret *corev1.Secret, cd *hivev1.ClusterDeployment, secretClient corev1client.SecretInterface) (*corev1.Secret, error) {
	installConfigData, err := yaml.Marshal(installConfig)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(installConfigSecret.Data[installConfigKey], installConfigData) {
		return installConfigSecret, nil
	}
	installConfigSecret.Data[installConfigKey] = installConfigData
	return secretClient.Update(installConfigSecret)
}

func getSSHKey(cd *hiveapi.ClusterDeployment, secretClient corev1client.SecretInterface) string {
	sshKeySecret, err := secretClient.Get(cd.Spec.SSHKey.Name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return string(sshKeySecret.Data["ssh-publickey"])
}

func getMachinePools(cdName string, machinePoolClient hivev1client.MachinePoolInterface) ([]*hivev1.MachinePool, error) {
	poolsList, err := machinePoolClient.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var pools []*hivev1.MachinePool
	for i, p := range poolsList.Items {
		if p.Spec.ClusterDeploymentRef.Name != cdName {
			continue
		}
		pools = append(pools, &poolsList.Items[i])
	}
	return pools, nil
}

func reconcileMachinePools(oldMachinePools []*hivev1.MachinePool, machinePools []*hivev1.MachinePool, cd *hivev1.ClusterDeployment, machinePoolClient hivev1client.MachinePoolInterface) ([]*hivev1.MachinePool, error) {
	for i, p := range machinePools {
		addOwnerRef(cd, p)
		existing := false
		for _, oldP := range oldMachinePools {
			if p.Name != oldP.Name {
				continue
			}
			if !reflect.DeepEqual(p, oldP) {
				var err error
				machinePools[i], err = machinePoolClient.Update(p)
				if err != nil {
					return machinePools, err
				}
			}
			existing = true
			break
		}
		if !existing {
			var err error
			machinePools[i], err = machinePoolClient.Create(p)
			if err != nil {
				return machinePools, err
			}
		}
	}
	for _, oldP := range oldMachinePools {
		found := false
		for _, p := range machinePools {
			if oldP.Name == p.Name {
				found = true
				break
			}
		}
		if !found {
			if err := machinePoolClient.Delete(oldP.Name, &metav1.DeleteOptions{}); err != nil {
				return machinePools, err
			}
		}
	}
	return machinePools, nil
}

func addOwnerRef(cd *hivev1.ClusterDeployment, obj metav1.Object) bool {
	return controller.EnsureOwnerRef(
		obj,
		metav1.OwnerReference{
			APIVersion:         hivev1.SchemeGroupVersion.String(),
			Kind:               "ClusterDeployment",
			Name:               cd.Name,
			UID:                cd.UID,
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	)
}
