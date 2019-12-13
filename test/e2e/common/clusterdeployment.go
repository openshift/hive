package common

import (
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/remoteclient"
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
	if !cd.Spec.Installed || cd.Spec.ClusterMetadata == nil || len(cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name) == 0 {
		log.WithField("clusterdeployment", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name)).Fatalf("cluster deployment is not installed")
	}
	return cd
}

func MustGetClusterDeploymentClientConfig() *rest.Config {
	cd := MustGetInstalledClusterDeployment()
	c := MustGetClient()
	remoteClientBuilder := remoteclient.NewBuilder(c, cd, "e2e-test")
	cfg, err := remoteClientBuilder.RESTConfig()
	if err != nil {
		log.WithError(err).Fatal("unable to get REST config for clusterdeployment")
		return nil
	}
	return cfg
}
