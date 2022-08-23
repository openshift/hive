package clusterprovision

import (
	"context"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	regexConfigMapName           = "install-log-regexes"
	additionalRegexConfigMapName = "additional-install-log-regexes"
	regexDataEntryName           = "regexes"
	unknownReason                = "UnknownError"
	logMissingMessage            = "Cluster install failed but installer log was not captured"
	regexBadMessage              = "Cluster install failed but regex configmap to parse for known reasons could not be used"
	unknownMessage               = "Cluster install failed but no known errors found in logs"
)

// parseInstallLog parses install log to monitor for known issues.
func (r *ReconcileClusterProvision) parseInstallLog(log *string, pLog log.FieldLogger) (string, string) {
	if log == nil {
		return unknownReason, logMissingMessage
	}

	// Load the regex configmap, if we don't have one, there's not much point proceeding here.
	// TODO: Should we expect the configmap in
	// - the control plane
	//   - with the deployment -- that's where we expect it today
	//   - with the operator -- it would be singular then, like the hiveconfig that uses it; but we would be changing the behavior
	// - the data plane -- but then the consumer would have to make sure to mirror the targetNamespace. This is what we're going
	//   with for now, as it fits with the principle of "data goes on the data plane". But I don't like it.
	regexCM := &corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: regexConfigMapName, Namespace: controllerutils.GetHiveNamespace()}, regexCM); err != nil {
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

	// Load additional regex configmap, continue anyway if configmap isn't present
	additionalRegexes := []installLogRegex{}
	additionalRegexCM := &corev1.ConfigMap{}
	if additionalRegexCMErr := r.Get(context.TODO(), types.NamespacedName{Name: additionalRegexConfigMapName, Namespace: controllerutils.GetHiveNamespace()}, additionalRegexCM); additionalRegexCMErr != nil {
		pLog.WithError(additionalRegexCMErr).Errorf("error loading %s configmap", additionalRegexConfigMapName)
	} else {
		additionalRegexesRaw, ok := additionalRegexCM.Data[regexDataEntryName]
		if !ok {
			pLog.Errorf("%s configmap does not have a %q data entry", additionalRegexConfigMapName, regexDataEntryName)
		} else {
			if additionalRegexesRaw != "" {
				if err := yaml.Unmarshal([]byte(additionalRegexesRaw), &additionalRegexes); err != nil {
					pLog.WithError(err).Errorf("cannot unmarshal data from %s configmap", regexConfigMapName)
				}
			}
		}
	}

	pLog.Info("processing new install log")

	// Log each line separately, this brings all our install logs from many namespaces into
	// the main hive log where we can aggregate search results.
	for _, l := range strings.Split(*log, "\n") {
		pLog.WithField("line", l).Info("install log line")
	}

	// Scan log contents for known errors
	combinedRegexes := append(regexes, additionalRegexes...)
	for _, ilr := range combinedRegexes {
		ilrLog := pLog.WithField("regexName", ilr.Name)
		ilrLog.Debug("parsing regex entry")
		for _, ss := range ilr.SearchRegexStrings {
			// Make the expression case insensitive.
			// NOTE: This works correctly on a regex that already has flaggage. E.g. "(?i)(?s)..."
			// is equivalent to "(?is)..."
			ss = "(?i)" + ss
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

	return unknownReason, *log
}
