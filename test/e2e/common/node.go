package common

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	clientcache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func WaitForNodes(cfg *rest.Config, testFunc func([]*corev1.Node) bool, timeOut time.Duration) error {
	logger := log.WithField("client", "node")
	logger.Infof("Waiting for Nodes")
	stop := make(chan struct{})
	done := make(chan struct{})
	internalCache, err := cache.New(cfg, cache.Options{})
	if err != nil {
		return err
	}
	informer, err := internalCache.GetInformer(&corev1.Node{})
	if err != nil {
		return err
	}
	onUpdate := func() {
		list := informer.GetStore().List()
		nodeList := []*corev1.Node{}
		for _, item := range list {
			node, ok := item.(*corev1.Node)
			if !ok {
				log.Fatalf("Item is not of type Node: %#v", item)
				continue
			}
			nodeList = append(nodeList, node)
		}
		if testFunc(nodeList) {
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
		return fmt.Errorf("timed out waiting for nodes")
	case <-done:
	}
	return nil
}
