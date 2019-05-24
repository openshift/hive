/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package installlogmonitor

import (
	"regexp"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// InstallLogRegex is a struct that represents all the data we use to scan for certain
// search strings in install logs. These structs are serialized as yaml and stored/read from
// the install-log-regexes ConfigMap.
type InstallLogRegex struct {
	// SearchRegexStrings are the regex strings we will search for.
	SearchRegexStrings []string `json:"searchRegexStrings"`

	// SearchRegexes are the compile regexes created from the SearchRegexStrings. This field is
	// set by the load code below, not parsed from json.
	SearchRegexes []*regexp.Regexp

	// InstallFailingReason is the single word CamelCase reason we report for this failure in conditions, metrics and logs.
	InstallFailingReason string `json:"installFailingReason"`

	// InstallFailingMessage is the user friendly sentence we report for this failure and conditions, metrics and logs.
	InstallFailingMessage string `json:"installFailingMessage"`
}

func loadInstallLogRegexes(regexesCM *corev1.ConfigMap, logger log.FieldLogger) ([]InstallLogRegex, error) {
	regexes := []InstallLogRegex{}
	for k, v := range regexesCM.Data {
		keyLog := logger.WithField("configMapKey", k)

		var r InstallLogRegex
		err := yaml.Unmarshal([]byte(v), &r)
		if err != nil {
			keyLog.WithError(err).WithField("data", v).Error("unable to deserialize yaml for InstallLogRegex")
			return regexes, err
		}
		// Compile all the search regex strings:
		r.SearchRegexes = []*regexp.Regexp{}
		for _, ss := range r.SearchRegexStrings {
			re, err := regexp.Compile(ss)
			if err != nil {
				logger.WithError(err).WithField("searchString", ss).Error("unable to compile regex")
				return regexes, err
			}
			r.SearchRegexes = append(r.SearchRegexes, re)

		}
		regexes = append(regexes, r)

	}
	return regexes, nil
}
