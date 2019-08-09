package util

import (
	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/resource"
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
