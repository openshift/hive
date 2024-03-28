package machinepool

import (
	"encoding/json"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	cpms "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/hive/pkg/util/logrus"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func FailureDomainFromProviderSpec(ms *machineapi.MachineSet, infrastructure *configv1.Infrastructure, logger log.FieldLogger) (failuredomain.FailureDomain, error) {

	mSpec := ms.Spec.Template.Spec
	var err error

	if mSpec.ProviderSpec.Value == nil {
		return nil, errors.New("remote MachineSet provider spec is nil")
	}
	// cpms utils operate on the Raw value of the provider spec rather than the Object,
	// and internally unmarshal the raw value into the Object.
	// Ensure that Raw is populated before calling cpms utils.
	// TODO remove this once cpms utils operate on the Object instead of the Raw value.
	if mSpec.ProviderSpec.Value.Raw == nil {
		mSpec.ProviderSpec.Value.Raw, err = json.Marshal(mSpec.ProviderSpec.Value.Object)
		if err != nil {
			logger.WithError(err).Errorf("unable to marshal MachineSet %v provider spec object to raw value", ms.Name)
			return nil, err
		}
	}
	logr := logrus.NewLogr(logger)
	ms_providerconfig, err := cpms.NewProviderConfigFromMachineSpec(logr, mSpec, infrastructure)
	if err != nil {
		logger.WithError(err).Errorf("unable to parse MachineSet %v provider config", ms.Name)
		return nil, err
	}
	return ms_providerconfig.ExtractFailureDomain(), nil
}
