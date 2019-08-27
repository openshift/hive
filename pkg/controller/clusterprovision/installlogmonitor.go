package clusterprovision

import (
	"context"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/hive/pkg/constants"
)

const (
	regexConfigMapName = "install-log-regexes"
	regexDataEntryName = "regexes"
	unknownReason      = "UnknownError"
	logMissingMessage  = "Cluster install failed but installer log was not captured"
	regexBadMessage    = "Cluster install failed but regex configmap to parse for known reasons could not be used"
	unknownMessage     = "Cluster install failed but no known errors found in logs"
)

// parseInstallLog parses install log to monitor for known issues.
func (r *ReconcileClusterProvision) parseInstallLog(log *string, pLog log.FieldLogger) (string, string) {
	if log == nil {
		return unknownReason, logMissingMessage
	}

	// Load the regex configmap, if we don't have one, there's not much point proceeding here.
	regexCM := &corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: regexConfigMapName, Namespace: constants.HiveNamespace}, regexCM); err != nil {
		pLog.WithError(err).Errorf("error loading %s configmap", regexConfigMapName)
		// Even if the error was a transient error in fetching the configmap, we should not block
		// the continuation of deploying the cluster just so that we can potentially get a
		// better failure message.
		return unknownReason, regexBadMessage
	}

	regexesRaw, ok := regexCM.Data[regexDataEntryName]
	if !ok {
		pLog.Errorf("%s configmap does not have a %q data entry", regexConfigMapName, regexDataEntryName)
		return unknownReason, regexBadMessage
	}

	regexes := []installLogRegex{}
	if err := yaml.Unmarshal([]byte(regexesRaw), &regexes); err != nil {
		pLog.WithError(err).Errorf("cannot unmarshal data from %s configmap", regexConfigMapName)
		return unknownReason, regexBadMessage
	}

	pLog.Info("processing new install log")

	// Log each line separately, this brings all our install logs from many namespaces into
	// the main hive log where we can aggregate search results.
	for _, l := range strings.Split(*log, "\n") {
		pLog.WithField("line", l).Info("install log line")
	}

	// Scan log contents for known errors
	for _, ilr := range regexes {
		ilrLog := pLog.WithField("regexName", ilr.Name)
		ilrLog.Debug("parsing regex entry")
		for _, ss := range ilr.SearchRegexStrings {
			ssLog := ilrLog.WithField("searchString", ss)
			ssLog.Debug("matching search string")
			switch match, err := regexp.Match(ss, []byte(*log)); {
			case err != nil:
				ssLog.WithError(err).Error("unable to compile regex")
			case match:
				pLog.WithField("reason", ilr.InstallFailingReason).Info("found known install failure string")
				return ilr.InstallFailingReason, ilr.InstallFailingMessage
			}
		}
	}

	return unknownReason, unknownMessage
}
