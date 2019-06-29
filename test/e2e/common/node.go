package common

import (
	"context"
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
		nodes := &corev1.NodeList{}
		internalCache.List(context.TODO(), nodes)
		nodeList := []*corev1.Node{}
		for i := range nodes.Items {
			nodeList = append(nodeList, &nodes.Items[i])
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
