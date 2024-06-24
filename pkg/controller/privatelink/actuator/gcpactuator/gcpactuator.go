package gcpactuator

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
)

var (
	requeueLater = reconcile.Result{RequeueAfter: 1 * time.Minute}
)

func initPrivateServiceConnectStatus(cd *hivev1.ClusterDeployment) {
	if cd.Status.Platform == nil {
		cd.Status.Platform = &hivev1.PlatformStatus{}
	}
	if cd.Status.Platform.GCP == nil {
		cd.Status.Platform.GCP = &hivev1gcp.PlatformStatus{}
	}
	if cd.Status.Platform.GCP.PrivateServiceConnect == nil {
		cd.Status.Platform.GCP.PrivateServiceConnect = &hivev1gcp.PrivateServiceConnectStatus{}
	}
}

func updatePrivateServiceConnectStatus(client *client.Client, cd *hivev1.ClusterDeployment) error {
	var retryBackoff = wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	return retry.RetryOnConflict(retryBackoff, func() error {
		curr := &hivev1.ClusterDeployment{}
		err := (*client).Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
		if err != nil {
			return err
		}

		initPrivateServiceConnectStatus(curr)
		curr.Status.Platform.GCP.PrivateServiceConnect = cd.Status.Platform.GCP.PrivateServiceConnect
		return (*client).Status().Update(context.TODO(), curr)
	})
}
