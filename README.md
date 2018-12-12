# OpenShift Hive
API driven OpenShift cluster provisioning and management

## Prerequisites

 * Install mockgen:
   * `$ go get github.com/golang/mock/gomock; go install github.com/golang/mock/mockgen`

## Deploying In-Cluster

* Ensure that you have access to an OpenShift cluster and have administrator permissions. This could be oc cluster up, minishift, or an actual cluster you can oc login to.
* Build and deploy to Minishift:
  * `$ hack/minishift-deploy.sh`
* Build and deploy to current kubectl context using local container image:
   * `$ make deploy`
* Build and deploy to current kubectl context using remote container image:
   * `$ make deploy-sd-dev`

## Running from Source

* Create the ClusterDeployment and DNSZone CRDs:
  * `$ kubectl apply -f config/crds/hive_v1alpha1_clusterdeployment.yaml`
  * `$ kubectl apply -f config/crds/hive_v1alpha1_dnszone.yaml`
* Run the Hive controllers from source:
  * `$ make run`

## Using Hive
* Create a ClusterDeployment using local container images:
  * Place the OpenShift images pull secret in a known location like `$HOME/config.json`
  * Assuming AWS credentials set in the standard environment variables, and our usual SSH key.
  ```bash
  export CLUSTER_NAME="${USER}"
  export ADMIN_EMAIL="${USER}@redhat.com"
  export ADMIN_PASSWORD="letmein"
  export SSH_PUB_KEY="$(ssh-keygen -y -f ~/.ssh/libra.pem)"
  export PULL_SECRET="$(cat ${HOME}/config.json)"

  oc process -f config/templates/cluster-deployment.yaml \
     CLUSTER_NAME="${CLUSTER_NAME}" \
     ADMIN_EMAIL="${ADMIN_EMAIL}" \
     ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
     SSH_KEY="${SSH_PUB_KEY}" \
     PULL_SECRET="${PULL_SECRET}" \
     AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
     AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
     | oc apply -f -
  ```
* Create a ClusterDeployment using remote container images:
  * Place the OpenShift images pull secret in a known location like `$HOME/config.json`
  * Assuming AWS credentials set in the standard environment variables, and our usual SSH key.
  ```bash
  export CLUSTER_NAME="${USER}"
  export ADMIN_EMAIL="${USER}@redhat.com"
  export ADMIN_PASSWORD="letmein"
  export SSH_PUB_KEY="$(ssh-keygen -y -f ~/.ssh/libra.pem)"
  export PULL_SECRET="$(cat ${HOME}/config.json)"
  export HIVE_IMAGE="quay.io/twiest/hive-controller:20181212"
  export HIVE_IMAGE_PULL_POLICY="Always"
  export INSTALLER_IMAGE="quay.io/twiest/installer:20181212"
  export INSTALLER_IMAGE_PULL_POLICY="Always"

  oc process -f config/templates/cluster-deployment.yaml \
     CLUSTER_NAME="${CLUSTER_NAME}" \
     ADMIN_EMAIL="${ADMIN_EMAIL}" \
     ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
     SSH_KEY="${SSH_PUB_KEY}" \
     PULL_SECRET="${PULL_SECRET}" \
     AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
     AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
     HIVE_IMAGE="${HIVE_IMAGE}" \
     HIVE_IMAGE_PULL_POLICY="${HIVE_IMAGE_PULL_POLICY}" \
     INSTALLER_IMAGE="${INSTALLER_IMAGE}" \
     INSTALLER_IMAGE_PULL_POLICY="${INSTALLER_IMAGE_PULL_POLICY}" \
     | oc apply -f -
  ```
* Delete your ClusterDeployment:
  ```bash
  $ oc delete clusterdeployment $USER
  ```

## Tips

### Using the Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
kubectl get secret ${USER}-admin-kubeconfig -o json | jq ".data.kubeconfig" -r | base64 -d > ${USER}.kubeconfig
export KUBECONFIG=${USER}.kubeconfig
kubectl get nodes
```

### Troubleshooting Deprovision

After deleting your cluster deployment you will see an uninstall job created. If for any reason this job gets stuck you can:

 1. Delete the uninstall job, it will be recreated and tried again.
 1. Manually delete the uninstall finalizer allowing the cluster deployment to be deleted, but note that this may leave artifacts in your AWS account.
    * `kubectl edit clusterdeployments.hive.openshift.io YOURCLUSTER`
 1. You can manually run the uninstall code with hiveutil to delete AWS resources based on their tags.
    * Get your cluster UUID from the clusterdeployment.Spec.ClusterUUID.
    * `make hiveutil`
    * `bin/hiveutil aws-tag-deprovision --loglevel=debug --cluster-name CLUSTER_NAME openshiftClusterID=CLUSTER_UUID kubernetes.io/cluster/CLUSTER_NAME=owned`
