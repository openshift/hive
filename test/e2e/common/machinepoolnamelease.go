package common

import (
	"context"
	"fmt"
	hiveclient "github.com/openshift/hive/pkg/client/clientset-generated/clientset"
	"github.com/openshift/hive/pkg/constants"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clientwatch "k8s.io/client-go/tools/watch"
	"time"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"
	clientcache "k8s.io/client-go/tools/cache"
)

// WaitForMachinePoolNameLease waits for a lease to appear in the given namespace for the given pool.
func WaitForMachinePoolNameLease(config *rest.Config, namespace, poolName string, timeOut time.Duration) (string, error) {
	logger := log.WithField("pool", poolName)
	logger.Infof("Waiting for MachinePoolNameLease")

	waitContext, cancel := context.WithTimeout(context.Background(), timeOut)
	defer cancel()

	hiveClient, err := hiveclient.NewForConfig(config)
	if err != nil {
		return "", errors.Wrap(err, "could not create the Hive client")
	}

	restClient := hiveClient.HiveV1().RESTClient()
	optionsModifier := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%s=%s", constants.MachinePoolNameLabel, poolName)
	}

	event, err := clientwatch.UntilWithSync(
		waitContext,
		clientcache.NewFilteredListWatchFromClient(
			restClient,
			"machinepoolnameleases",
			namespace,
			optionsModifier,
		),
		&hivev1.MachinePoolNameLease{},
		nil,
		func(e watch.Event) (bool, error) {
			return e.Type == watch.Added || e.Type == watch.Modified, nil
		},
	)
	if err != nil {
		return "", errors.Wrap(err, "timeout waiting for MachinePoolNameLease")
	}
	lease, ok := event.Object.(*hivev1.MachinePoolNameLease)
	if !ok {
		return "", fmt.Errorf("expected a MachinePoolNameLease object but got a %T object instead", event.Object)
	}
	logger.Infof("found MachinePoolName lease: %s", lease.Name)
	return lease.Name, nil
}
