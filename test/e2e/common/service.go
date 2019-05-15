package common

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func WaitForService(c kclient.Interface, namespace, name string, testFunc func(*corev1.Service) bool, timeOut time.Duration) error {
	logger := log.WithField("service", fmt.Sprintf("%s/%s", namespace, name))
	logger.Infof("Waiting for service")
	stop := make(chan struct{})
	done := make(chan struct{})
	onObject := func(obj interface{}) {
		service, ok := obj.(*corev1.Service)
		if !ok {
			logger.Warningf("object not service: %v", obj)
			return
		}
		if testFunc(service) {
			done <- struct{}{}
		}
	}
	watchList := cache.NewListWatchFromClient(c.CoreV1().RESTClient(), "services", namespace, fields.OneTermEqualSelector("metadata.name", name))
	_, controller := cache.NewInformer(
		watchList,
		&corev1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: onObject,
			UpdateFunc: func(oldObject, newObject interface{}) {
				onObject(newObject)
			},
		},
	)
	go controller.Run(stop)
	defer func() { stop <- struct{}{} }()

	select {
	case <-time.After(timeOut):
		return fmt.Errorf("timed out waiting for service %s/%s", namespace, name)
	case <-done:
	}
	return nil
}
