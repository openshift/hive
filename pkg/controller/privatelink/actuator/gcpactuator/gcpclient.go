package gcpactuator

import (
	"context"
	"net/http"

	"google.golang.org/api/googleapi"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/gcpclient"
)

type gcpClientFn func(secret *corev1.Secret) (gcpclient.Client, error)

func newGCPClient(client client.Client, clientFn gcpClientFn, secretName string, secretNamespace string) (gcpclient.Client, error) {
	if clientFn == nil {
		clientFn = gcpclient.NewClientFromSecret
	}

	secret := &corev1.Secret{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		secret)
	if err != nil {
		return nil, err
	}
	return clientFn(secret)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	ae, ok := err.(*googleapi.Error)
	return ok && (ae.Code == http.StatusNotFound)
}
