package common

import (
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func MustGetClusterDeployment() *hivev1.ClusterDeployment {
	name := os.Getenv("CLUSTER_NAME")
	if len(name) == 0 {
		log.Fatalf("No cluster name was specified. Use the CLUSTER_NAME environment variable to specify a cluster deployment")
	}
	namespace := os.Getenv("CLUSTER_NAMESPACE")
	if len(namespace) == 0 {
		log.Fatalf("No cluster namespace was specified. Use the CLUSTER_NAMESPACE environment variable to specify a namespace")
	}
	c := MustGetClient()
	cd := &hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, cd)
	if err != nil {
		log.WithError(err).Fatalf("Error fetching cluster deployment")
	}
	return cd
}

func MustGetInstalledClusterDeployment() *hivev1.ClusterDeployment {
	cd := MustGetClusterDeployment()
	if !cd.Spec.Installed || len(cd.Status.AdminKubeconfigSecret.Name) == 0 {
		log.WithField("clusterdeployment", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name)).Fatalf("cluster deployment is not installed")
	}
	return cd
}

func MustGetClusterDeploymentClientConfig() *rest.Config {
	cd := MustGetInstalledClusterDeployment()
	c := MustGetClient()
	adminKubeconfigSecret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: cd.Status.AdminKubeconfigSecret.Name, Namespace: cd.Namespace}, adminKubeconfigSecret)
	if err != nil {
		log.WithError(err).Fatal("unable to fetch admin kubeconfig secret")
		return nil
	}
	kubeConfig, err := controllerutils.FixupKubeconfigSecretData(adminKubeconfigSecret.Data)
	if err != nil {
		log.WithError(err).Fatal("unable to fixup admin kubeconfig")
		return nil
	}
	config, err := clientcmd.Load([]byte(kubeConfig))
	if err != nil {
		log.WithError(err).Fatal("unable to load kubeconfig")
		return nil
	}
	cfg, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.WithError(err).Fatal("unable to load kubeconfig")
		return nil
	}
	return cfg
}
