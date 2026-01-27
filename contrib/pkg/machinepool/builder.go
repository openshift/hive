package machinepool

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

type PlatformActuator interface {
	Build(cd *hivev1.ClusterDeployment, config *MachinePoolConfig, mp *hivev1.MachinePool) error
}

type UnifiedPlatformActuator struct {
	platform string
}

func NewUnifiedPlatformActuator(platform string) *UnifiedPlatformActuator {
	return &UnifiedPlatformActuator{platform: platform}
}

func (a *UnifiedPlatformActuator) Build(cd *hivev1.ClusterDeployment, config *MachinePoolConfig, mp *hivev1.MachinePool) error {
	handler, err := GetPlatformHandler(a.platform)
	if err != nil {
		return errors.Wrapf(err, "unsupported platform: %s", a.platform)
	}
	if config.CreateOptions != nil {
		return handler.BuildPlatformStruct(config.CreateOptions, mp)
	}
	if config.AdoptData != nil && config.AdoptData.Config.ProviderConfig != nil {
		return handler.BuildPlatformStructFromProviderConfig(config.AdoptData.Config.ProviderConfig, config.AdoptData, mp)
	}
	// Neither mode is properly configured
	return errors.Errorf("cannot build MachinePool: either CreateOptions (for create mode) or AdoptData with ProviderConfig (for adopt mode) must be provided")
}

type MachinePoolConfig struct {
	ClusterDeployment *hivev1.ClusterDeployment
	PoolName          string
	Replicas          *int64
	Platform          string
	Infrastructure    any
	AdoptData         *MachineSetAnalysis
	CreateOptions     *CreateOptions
	Labels            map[string]string
	MachineLabels     map[string]string
	Taints            []corev1.Taint
	Autoscaling       *hivev1.MachinePoolAutoscaling
}

func NewMachinePoolConfigFromAnalysis(cd *hivev1.ClusterDeployment, poolName string, analysis *MachineSetAnalysis) *MachinePoolConfig {
	replicas := int64(analysis.Summary.TotalReplicas)
	return &MachinePoolConfig{
		ClusterDeployment: cd,
		PoolName:          poolName,
		Replicas:          &replicas,
		Platform:          analysis.Summary.Platform,
		Infrastructure:    analysis.Input.Infrastructure,
		AdoptData:         analysis,
	}
}

func BuildMachinePool(config *MachinePoolConfig) (*hivev1.MachinePool, error) {
	builder, err := NewMachinePoolBuilderFromConfig(config)
	if err != nil {
		return nil, err
	}
	return builder.Build()
}

type MachinePoolBuilder struct {
	cd       *hivev1.ClusterDeployment
	config   *MachinePoolConfig
	actuator *UnifiedPlatformActuator
}

func NewMachinePoolBuilder(cd *hivev1.ClusterDeployment, poolName string, analysis *MachineSetAnalysis, logger log.FieldLogger) (*MachinePoolBuilder, error) {
	return NewMachinePoolBuilderFromConfig(NewMachinePoolConfigFromAnalysis(cd, poolName, analysis))
}

func NewMachinePoolBuilderFromConfig(config *MachinePoolConfig) (*MachinePoolBuilder, error) {
	switch {
	case config == nil:
		return nil, errors.New("MachinePoolConfig is required")
	case config.Platform == "":
		return nil, errors.New("Platform is required in MachinePoolConfig")
	}
	return &MachinePoolBuilder{
		cd:       config.ClusterDeployment,
		config:   config,
		actuator: NewUnifiedPlatformActuator(config.Platform),
	}, nil
}

func (b *MachinePoolBuilder) Build() (*hivev1.MachinePool, error) {
	switch {
	case b.config.ClusterDeployment == nil:
		return nil, errors.New("ClusterDeployment is required")
	case b.config.PoolName == "":
		return nil, errors.New("PoolName is required")
	}

	mp := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetMachinePoolName(b.config.ClusterDeployment.Name, b.config.PoolName),
			Namespace: b.config.ClusterDeployment.Namespace,
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{Name: b.config.ClusterDeployment.Name},
			Name:                 b.config.PoolName,
			Replicas:             b.config.Replicas,
			Labels:               b.config.Labels,
			MachineLabels:        b.config.MachineLabels,
			Taints:               b.config.Taints,
			Autoscaling:          b.config.Autoscaling,
		},
	}

	if err := b.actuator.Build(b.cd, b.config, mp); err != nil {
		return nil, errors.Wrapf(err, "failed to build %s platform config", b.config.Platform)
	}
	return mp, nil
}
