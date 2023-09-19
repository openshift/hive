package machinepool

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	openstackproviderv1alpha1 "sigs.k8s.io/cluster-api-provider-openstack/pkg/apis/openstackproviderconfig/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installosp "github.com/openshift/installer/pkg/asset/machines/openstack"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesosp "github.com/openshift/installer/pkg/types/openstack"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// OpenStackActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type OpenStackActuator struct {
	logger     log.FieldLogger
	osImage    string
	kubeClient client.Client
}

var _ Actuator = &OpenStackActuator{}

// NewOpenStackActuator is the constructor for building a OpenStackActuator
func NewOpenStackActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, kubeClient client.Client, logger log.FieldLogger) (*OpenStackActuator, error) {
	osImage, err := getOpenStackOSImage(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting os image from master machine")
		return nil, err
	}
	actuator := &OpenStackActuator{
		logger:     logger,
		osImage:    osImage,
		kubeClient: kubeClient,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *OpenStackActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.OpenStack == nil {
		return nil, false, errors.New("ClusterDeployment is not for OpenStack")
	}
	if pool.Spec.Platform.OpenStack == nil {
		return nil, false, errors.New("MachinePool is not for OpenStack")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.OpenStack = &installertypesosp.MachinePool{
		FlavorName: pool.Spec.Platform.OpenStack.Flavor,
		// The installer's MachinePool-to-MachineSet function will distribute the generated
		// MachineSets across the list of Zones. As we don't presently support defining zones
		// in Hive MachinePools, make sure we send at least a list of one zone so that we
		// get back a MachineSet.
		// Providing the empty string will give back a MachineSet running on the default
		// OpenStack Nova availability zone.
		Zones: []string{""},
	}

	if pool.Spec.Platform.OpenStack.RootVolume != nil {
		computePool.Platform.OpenStack.RootVolume = &installertypesosp.RootVolume{
			Size: pool.Spec.Platform.OpenStack.RootVolume.Size,
			Types: []string{
				pool.Spec.Platform.OpenStack.RootVolume.Type,
			},
		}
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			OpenStack: &installertypesosp.Platform{
				Cloud: cd.Spec.Platform.OpenStack.Cloud,
			},
		},
	}

	credsSecretKey := types.NamespacedName{
		Name:      cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name,
		Namespace: cd.Namespace,
	}
	yamlOpts, err := newYamlOptsBuilder(a.kubeClient, credsSecretKey)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to create yamlOpts for openstack client")
	}

	clientOptions := &clientconfig.ClientOpts{
		Cloud:    cd.Spec.Platform.OpenStack.Cloud,
		YAMLOpts: yamlOpts,
	}

	if cd.Spec.Platform.OpenStack.CertificatesSecretRef != nil {
		buf := &bytes.Buffer{}
		if err := controllerutils.TrustBundleFromSecretToWriter(a.kubeClient, cd.Namespace, cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name, buf); err != nil {
			return nil, false, errors.Wrap(err, "failed to load trust bundle from CertificatesSecretRef")
		}
		if err := yamlOpts.updateTrust(clientOptions.Cloud, buf.Bytes()); err != nil {
			return nil, false, errors.Wrap(err, "failed to update trust in the yamlOpts")
		}
		clientOptions.YAMLOpts = yamlOpts
	}

	installerMachineSets, err := installosp.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		a.osImage,
		workerRole,
		workerUserDataName,
		clientOptions,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

// Get the OS image from an existing master machine.
func getOpenStackOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeOpenStackMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
	if err != nil {
		logger.WithError(err).Warn("cannot decode OpenstackProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode OpenstackProviderSpec from master machine")
	}
	var osImage string
	if providerSpec.RootVolume != nil {
		osImage = providerSpec.RootVolume.SourceUUID
	} else {
		osImage = providerSpec.Image
	}
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

func decodeOpenStackMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*openstackproviderv1alpha1.OpenstackProviderSpec, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(openstackproviderv1alpha1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode OpenStack ProviderSpec: %v", err)
	}
	spec, ok := obj.(*openstackproviderv1alpha1.OpenstackProviderSpec)
	if !ok {
		return nil, fmt.Errorf("unexpected object: %#v", gvk)
	}
	return spec, nil
}

// yamlOptsBuilder lets us provide our own functions to return a 'clouds.yaml' file that has been
// unmarshaled into the format expected by the OpenStack clients.
type yamlOptsBuilder struct {
	cloudYaml map[string]clientconfig.Cloud
}

func newYamlOptsBuilder(kubeClient client.Client, credsSecretKey types.NamespacedName) (*yamlOptsBuilder, error) {

	credsSecret := &corev1.Secret{}
	if err := kubeClient.Get(context.TODO(), credsSecretKey, credsSecret); err != nil {
		return nil, errors.Wrap(err, "failed to get OpenStack credentials")
	}

	cloudsYaml, ok := credsSecret.Data[constants.OpenStackCredentialsName]
	if !ok {
		return nil, errors.New("did not find credentials in the OpenStack credentials secret")
	}

	var clouds clientconfig.Clouds
	if err := yaml.Unmarshal(cloudsYaml, &clouds); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal yaml stored in secret")
	}

	optsBuilder := &yamlOptsBuilder{
		cloudYaml: clouds.Clouds,
	}
	return optsBuilder, nil
}

func (opts *yamlOptsBuilder) LoadCloudsYAML() (map[string]clientconfig.Cloud, error) {
	return opts.cloudYaml, nil
}

func (opts *yamlOptsBuilder) LoadSecureCloudsYAML() (map[string]clientconfig.Cloud, error) {
	// secure.yaml is optional so just pretend it doesn't exist
	return nil, nil
}

func (opts *yamlOptsBuilder) LoadPublicCloudsYAML() (map[string]clientconfig.Cloud, error) {
	return nil, fmt.Errorf("LoadPublicCloudsYAML() not implemented")
}

func (opts *yamlOptsBuilder) updateTrust(cloud string, trust []byte) error {
	conf, ok := opts.cloudYaml[cloud]
	if !ok {
		return errors.Errorf("no cloud %s found", cloud)
	}
	conf.CACertFile = string(trust)
	opts.cloudYaml[cloud] = conf
	return nil
}
