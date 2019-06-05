package common

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	apiregv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	apiregv1client "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"
)

func WaitForAPIServiceAvailable(c apiregv1client.ApiregistrationV1Interface, name string, timeOut time.Duration) error {
	readyFunc := func(s *apiregv1.APIService) bool {
		logger := log.WithField("apiservice", s.Name)
		logger.Debug("determining if apiservice is ready")
		for _, condition := range s.Status.Conditions {
			if condition.Type == apiregv1.Available {
				if condition.Status == apiregv1.ConditionTrue {
					logger.Debug("apiservice is ready")
					return true
				}
			}
		}
		logger.Debug("apiservice is not yet ready")
		return false
	}
	return WaitForAPIService(c, name, readyFunc, timeOut)
}

func WaitForAPIService(c apiregv1client.ApiregistrationV1Interface, name string, testFunc func(*apiregv1.APIService) bool, timeOut time.Duration) error {
	logger := log.WithField("APIService", name)
	logger.Infof("Waiting for APIService")
	stop := make(chan struct{})
	done := make(chan struct{})
	onObject := func(obj interface{}) {
		apiService, ok := obj.(*apiregv1.APIService)
		if !ok {
			logger.Warningf("object not APIService: %v", obj)
			return
		}
		if testFunc(apiService) {
			done <- struct{}{}
		}
	}
	watchList := cache.NewListWatchFromClient(c.RESTClient(), "apiservices", "", fields.OneTermEqualSelector("metadata.name", name))
	_, controller := cache.NewInformer(
		watchList,
		&apiregv1.APIService{},
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
		return fmt.Errorf("timed out waiting for apiservice %s", name)
	case <-done:
	}
	return nil
}
