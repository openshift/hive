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
	done := make(chan struct{})
	internalCache, err := cache.New(cfg, cache.Options{})
	if err != nil {
		return err
	}
	informer, err := internalCache.GetInformer(context.Background(), &corev1.Node{})
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
			AddFunc:    func(obj any) { onUpdate() },
			UpdateFunc: func(oldObj, newObj any) { onUpdate() },
			DeleteFunc: func(obj any) { onUpdate() },
		})

	ctx, stop := context.WithCancel(context.Background())
	go internalCache.Start(ctx)
	defer stop()

	select {
	case <-time.After(timeOut):
		return fmt.Errorf("timed out waiting for nodes")
	case <-done:
	}
	return nil
}
