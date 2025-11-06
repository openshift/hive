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

func WaitForMachineSets(cfg *rest.Config, testFunc func([]*machinev1.MachineSet) bool, timeOut time.Duration) error {
	logger := log.WithField("client", "machineset")
	logger.Infof("Waiting for MachineSet")
	done := make(chan struct{})

	internalCache, err := cache.New(cfg, cache.Options{
		Scheme: scheme.GetScheme(),
	})
	if err != nil {
		return err
	}
	informer, err := internalCache.GetInformer(context.Background(), &machinev1.MachineSet{})
	if err != nil {
		return err
	}
	onUpdate := func() {
		machineSets := &machinev1.MachineSetList{}
		internalCache.List(context.TODO(), machineSets)
		machineSetList := []*machinev1.MachineSet{}
		for i := range machineSets.Items {
			machineSetList = append(machineSetList, &machineSets.Items[i])
		}
		if testFunc(machineSetList) {
			done <- struct{}{}
		}
	}
	informer.AddEventHandler(
		&clientcache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj any) { onUpdate() },
			UpdateFunc: func(oldObj, newObj any) { onUpdate() },
			DeleteFunc: func(obj any) { onUpdate() },
		})

	ctx, stop := context.WithCancel(context.Background())
	go internalCache.Start(ctx)
	defer stop()

	select {
	case <-time.After(timeOut):
		return fmt.Errorf("timed out waiting for machinesets")
	case <-done:
	}
	return nil
}
