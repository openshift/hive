/*
Copyright 2018 The Kubernetes Authors.

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

package util

import (
	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
)

const (
	tokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// ApplyAsset loads a path from our bindata assets and applies it to the cluster.
func ApplyAsset(h *resource.Helper, assetPath string, hLog log.FieldLogger) error {
	assetLog := hLog.WithField("asset", assetPath)
	assetLog.Debug("reading asset")
	asset := assets.MustAsset(assetPath)
	assetLog.Debug("applying asset")
	result, err := h.Apply(asset)
	if err != nil {
		assetLog.WithError(err).Error("error applying asset")
		return err
	}
	assetLog.Infof("asset applied successfully: %v", result)
	return nil
}
