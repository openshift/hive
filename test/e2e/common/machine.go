package common

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"
	clientcache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/hive/pkg/util/scheme"
)

func WaitForMachines(cfg *rest.Config, testFunc func([]*machinev1.Machine) bool, timeOut time.Duration) error {
	logger := log.WithField("client", "machine")
	logger.Infof("Waiting for Machine")
	done := make(chan struct{})
	scheme := scheme.GetScheme()

	internalCache, err := cache.New(cfg, cache.Options{
		DefaultNamespaces: map[string]cache.Config{"openshift-machine-api": {}},
		Scheme:            scheme,
	})
	if err != nil {
		return err
	}
	informer, err := internalCache.GetInformer(context.Background(), &machinev1.Machine{})
	if err != nil {
		return err
	}
	onUpdate := func() {
		machines := &machinev1.MachineList{}
		internalCache.List(context.TODO(), machines)
		machineList := []*machinev1.Machine{}
		for i := range machines.Items {
			machineList = append(machineList, &machines.Items[i])
		}
		if testFunc(machineList) {
			done <- struct{}{}
		}
	}
	informer.AddEventHandler(
		&clientcache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { onUpdate() },
			UpdateFunc: func(oldObj, newObj interface{}) { onUpdate() },
			DeleteFunc: func(obj interface{}) { onUpdate() },
		})

	ctx, stop := context.WithCancel(context.Background())
	go internalCache.Start(ctx)
	defer stop()

	select {
	case <-time.After(timeOut):
		return fmt.Errorf("timed out waiting for machines")
	case <-done:
	}
	return nil
}
