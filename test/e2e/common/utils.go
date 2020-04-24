package common

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/hive"
)

const (
	dynamicDeleteTimeout = time.Second * 30
)

func GetHiveNamespaceOrDie() string {
	hiveNS := os.Getenv(constants.HiveNamespaceEnvVar)
	if len(hiveNS) == 0 {
		log.Fatalf("required environment variable is not defined: %s", constants.HiveNamespaceEnvVar)
	}
	return hiveNS
}

func GetHiveOperatorNamespaceOrDie() string {
	namespace := os.Getenv(hive.HiveOperatorNamespaceEnvVar)
	if len(namespace) == 0 {
		log.Fatalf("required environment variable is not defined: %s", hive.HiveOperatorNamespaceEnvVar)
	}
	return namespace
}

// DynamicWaitForDeletion uses the dynamic client to wait for a resource to not exist.
func DynamicWaitForDeletion(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string, logger log.FieldLogger) error {
	rLog := logger.WithFields(log.Fields{
		"gvr":       gvr,
		"namespace": namespace,
		"name":      name,
	})

	for start := time.Now(); ; {
		if time.Since(start) > dynamicDeleteTimeout {
			rLog.Error("resource not deleted before timeout")
			return fmt.Errorf("resource not deleted before timeout")
		}
		_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// resource does not exist
			rLog.Info("resource successfully deleted")
			return nil
		} else if err != nil {
			rLog.WithError(err).Info("unexpected error getting resource")
		} else {
			rLog.Info("resource still exists, sleeping...")
		}

		time.Sleep(500 * time.Millisecond)
	}
}
