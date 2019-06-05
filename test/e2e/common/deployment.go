package common

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func WaitForDeploymentReady(c kclient.Interface, namespace, name string, timeOut time.Duration) error {
	readyFunc := func(d *appsv1.Deployment) bool {
		logger := log.WithField("deployment", fmt.Sprintf("%s/%s", d.Namespace, d.Name))
		logger.Debug("determining if deployment is ready")
		for _, condition := range d.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable {
				if condition.Status == corev1.ConditionTrue {
					logger.Debug("deployment is ready")
					return true
				}
			}
		}
		logger.Debug("deployment is not yet ready")
		return false
	}
	return WaitForDeployment(c, namespace, name, readyFunc, timeOut)
}

func WaitForDeployment(c kclient.Interface, namespace, name string, testFunc func(*appsv1.Deployment) bool, timeOut time.Duration) error {
	logger := log.WithField("deployment", fmt.Sprintf("%s/%s", namespace, name))
	logger.Infof("Waiting for deployment")
	stop := make(chan struct{})
	done := make(chan struct{})
	onObject := func(obj interface{}) {
		deployment, ok := obj.(*appsv1.Deployment)
		if !ok {
			logger.Warningf("object not deployment: %v", obj)
			return
		}
		if testFunc(deployment) {
			done <- struct{}{}
		}
	}
	watchList := cache.NewListWatchFromClient(c.AppsV1().RESTClient(), "deployments", namespace, fields.OneTermEqualSelector("metadata.name", name))
	_, controller := cache.NewInformer(
		watchList,
		&appsv1.Deployment{},
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
		return fmt.Errorf("timed out waiting for deployment %s/%s", namespace, name)
	case <-done:
	}
	return nil
}
