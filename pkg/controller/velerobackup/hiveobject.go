package velerobackup

import (
	"fmt"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	errChecksum = "HIVE_CHECKSUM_ERR_97A29D08"
)

type hiveObject struct {
	checksum string
	object   runtime.Object
	logger   log.FieldLogger
}

func newHiveObject(object runtime.Object, logger log.FieldLogger) (*hiveObject, error) {
	var meta *metav1.ObjectMeta
	var spec interface{}

	switch t := object.(type) {
	case *hivev1.ClusterDeployment:
		meta = &t.ObjectMeta
		spec = &t.Spec
	case *hivev1.SyncSet:
		meta = &t.ObjectMeta
		spec = &t.Spec
	case *hivev1.DNSZone:
		meta = &t.ObjectMeta
		spec = &t.Spec
	default:
		return nil, fmt.Errorf("Unknown Type: %T", object)
	}

	return &hiveObject{
		checksum: calculateChecksum(meta, spec, logger),
		object:   object,
		logger:   logger,
	}, nil

}

func (h *hiveObject) hasChanged() bool {
	// Make sure the new checksum is different than the old checksum.
	return h.checksum != h.getChecksumAnnotation()
}

func calculateChecksum(meta *metav1.ObjectMeta, spec interface{}, logger log.FieldLogger) string {
	// We need to take the checksum WITHOUT the previous checksum
	// to avoid an infinite loop of the checksum always changing.
	metaCopy := meta.DeepCopy()
	delete(metaCopy.Annotations, controllerutils.LastBackupAnnotation)

	// We also need to take the checksum WITHOUT the previous ResourceVersion
	// as when we update the checksum annotation, ResourceVersion changes.
	metaCopy.ResourceVersion = ""

	checksum, err := controllerutils.GetChecksumOfObjects(metaCopy, spec)
	if err != nil {
		// We never return an error from this function as that could lead to a
		// situation where other objects change, but because one of the object
		// checksum errored, then the other objects won't get backed up.
		// Instead we return a constant checksum value. This function should rarely,
		// if ever, return an error.
		logger.WithError(err).Info("error calculating object checksum")
		return errChecksum
	}

	return checksum
}

func (h *hiveObject) getObjectMetadata() metav1.Object {
	tmpmeta, ok := h.object.(metav1.Object)
	if !ok {
		// This should never happen because we should always be an object.
		h.addLoggerFields().Error("Failed casting runtime object to metav1.Object")
		return nil
	}

	return tmpmeta
}

func (h *hiveObject) getChecksumAnnotation() string {
	tmpmeta := h.getObjectMetadata()
	if tmpmeta == nil {
		// We errored getting metadata.
		return ""
	}

	annotations := tmpmeta.GetAnnotations()
	return annotations[controllerutils.LastBackupAnnotation]
}

func (h *hiveObject) setChecksumAnnotation() {
	tmpmeta := h.getObjectMetadata()
	if tmpmeta == nil {
		// We errored getting metadata.
		return
	}

	annotations := tmpmeta.GetAnnotations()

	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Actually set the checksum
	annotations[controllerutils.LastBackupAnnotation] = h.checksum
	tmpmeta.SetAnnotations(annotations)
}

func (h *hiveObject) addLoggerFields() log.FieldLogger {
	fields := log.Fields{
		"gvk": h.object.GetObjectKind().GroupVersionKind(),
	}

	tmpmeta := h.getObjectMetadata()
	if tmpmeta != nil {
		fields["name"] = tmpmeta.GetName()
		fields["namespace"] = tmpmeta.GetNamespace()
	}

	return h.logger.WithFields(fields)
}
