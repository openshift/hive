package common

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"
	clientcache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
)

func WaitForMachineSets(cfg *rest.Config, testFunc func([]*machinev1.MachineSet) bool, timeOut time.Duration) error {
	logger := log.WithField("client", "machineset")
	logger.Infof("Waiting for MachineSet")
	stop := make(chan struct{})
	done := make(chan struct{})
	scheme, err := machinev1.SchemeBuilder.Build()
	if err != nil {
		return err
	}
	internalCache, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}
	informer, err := internalCache.GetInformer(&machinev1.MachineSet{})
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
			AddFunc:    func(obj interface{}) { onUpdate() },
			UpdateFunc: func(oldObj, newObj interface{}) { onUpdate() },
			DeleteFunc: func(obj interface{}) { onUpdate() },
		})

	go internalCache.Start(stop)
	defer func() { stop <- struct{}{} }()

	select {
	case <-time.After(timeOut):
		return fmt.Errorf("timed out waiting for machinesets")
	case <-done:
	}
	return nil
}
