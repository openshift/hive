- [OpenShift Hive](#openshift-hive)
  - [Prerequisites](#prerequisites)
  - [Deployment Options](#deployment-options)
    - [Deploy Hive Operator Using Latest Master Images](#deploy-hive-operator-using-latest-master-images)
    - [Deploy Hive Operator Using Custom Images](#deploy-hive-operator-using-custom-images)
    - [Deploy Hive via OLM](#deploy-hive-via-olm)
    - [Run Hive Operator From Source](#run-hive-operator-from-source)
    - [Run Hive From Source](#run-hive-from-source)
  - [Using Hive](#using-hive)
    - [Create a ClusterDeployment using the latest OpenShift release and installer image](#create-a-clusterdeployment-using-the-latest-openshift-release-and-installer-image)
    - [Create a ClusterDeployment using latest pinned and known stable container images](#create-a-clusterdeployment-using-latest-pinned-and-known-stable-container-images)
    - [Delete your ClusterDeployment](#delete-your-clusterdeployment)
  - [Tips](#tips)
    - [Using the Admin Kubeconfig](#using-the-admin-kubeconfig)
    - [Troubleshooting Deprovision](#troubleshooting-deprovision)
    - [Troubleshooting HiveAdmission](#troubleshooting-hiveadmission)
    - [Installing Federation](#installing-federation)
    - [Federation Example](#federation-example)
    - [Deploy using Minishift](#deploy-using-minishift)

# OpenShift Hive
API driven OpenShift cluster provisioning and management

## Prerequisites

 * mockgen:
   * `$ go get github.com/golang/mock/gomock; go install github.com/golang/mock/mockgen`
* [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* [oc](https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/)

## Deployment Options

Hive contains an operator which is responsible for handling deployment logic for the rest of the components. In the near future this operator may be installable via OLM.

### Deploy Hive Operator Using Latest Master Images

To deploy the operator from a git checkout:

  `$ make deploy`

By default the operator will use the latest images published by CI from the master branch.

You should now see hive-operator, hive-controllers, and hiveadmission pods running in the hive namespace.

### Deploy Hive Operator Using Custom Images

 1. Build and publish a custom Hive image from your current working dir: `$ IMG=quay.io/dgoodwin/hive:latest make buildah-push`
 2. Deploy with your custom image: `$ DEPLOY_IMAGE=quay.io/dgoodwin/hive:latest make deploy`

### Deploy Hive via OLM

We do not currently publish an official OLM operator package, but you can run or work off the test script below to generate a ClusterServiceVersion, OLM bundle+package, registry image, catalog source, and subscription.

`$ REGISTRY_IMG="quay.io/dgoodwin/hive-registry" DEPLOY_IMG="quay.io/dgoodwin/hive:latest" hack/olm-registry-deploy.sh`


### Run Hive Operator From Source

NOTE: assumes you have previously deployed using one of the above methods.

 1. `$ kubectl scale -n hive deployment.v1.apps/hive-operator --replicas=0`
 1. `$ make run-operator`

### Run Hive From Source

NOTE: assumes you have previously deployed using one of the above methods.

 1. `$ kubectl scale -n hive deployment.v1.apps/hive-controllers --replicas=0`
 1. `$ make run`

## Using Hive

1. Obtain a pull secret from try.openshift.com and place in a known location like `$HOME/config.json`.
1. **WARNING:** The template parameter BASE_DOMAIN (which defaults to "new-installer.openshift.com") **must** be different than the DNS base domain for the Hive cluster itself. For example, if the Hive cluster's DNS base domain is "foo.example.com", then BASE_DOMAIN **must** be set to something other than "foo.example.com". This will soon be fixed in the installer and no longer a requirement.
1. Ensure your AWS credentials are set in the normal environment variables: AWS_ACCESS_KEY_ID & AWS_SECRET_ACCESS_KEY
1. Note the use of an SSH key below to access the instances if necessary. (this should typically not be required)

### Create a ClusterDeployment using the latest OpenShift release and installer image

```bash
export CLUSTER_NAME="${USER}"
export SSH_PUB_KEY="$(ssh-keygen -y -f ~/.ssh/libra.pem)"
export PULL_SECRET="$(cat ${HOME}/config.json)"

oc process -f config/templates/cluster-deployment.yaml \
   CLUSTER_NAME="${CLUSTER_NAME}" \
   SSH_KEY="${SSH_PUB_KEY}" \
   PULL_SECRET="${PULL_SECRET}" \
   AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
   AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
   | oc apply -f -
```

### Create a ClusterDeployment using latest pinned and known stable container images

```bash
export CLUSTER_NAME="${USER}"
export SSH_PUB_KEY="$(ssh-keygen -y -f ~/.ssh/libra.pem)"
export PULL_SECRET="$(cat ${HOME}/config.json)"
export HIVE_IMAGE="quay.io/twiest/hive-controller:20190128"
export HIVE_IMAGE_PULL_POLICY="Always"
export INSTALLER_IMAGE="quay.io/twiest/installer:20190128"
export INSTALLER_IMAGE_PULL_POLICY="Always"
export RELEASE_IMAGE="quay.io/openshift-release-dev/ocp-release:4.0.0-0.1"

oc process -f config/templates/cluster-deployment.yaml \
   CLUSTER_NAME="${CLUSTER_NAME}" \
   SSH_KEY="${SSH_PUB_KEY}" \
   PULL_SECRET="${PULL_SECRET}" \
   AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
   AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
   HIVE_IMAGE="${HIVE_IMAGE}" \
   HIVE_IMAGE_PULL_POLICY="${HIVE_IMAGE_PULL_POLICY}" \
   INSTALLER_IMAGE="${INSTALLER_IMAGE}" \
   INSTALLER_IMAGE_PULL_POLICY="${INSTALLER_IMAGE_PULL_POLICY}" \
   RELEASE_IMAGE="${RELEASE_IMAGE}" \
   | oc apply -f -
```


### Delete your ClusterDeployment

```bash
$ oc delete clusterdeployment $USER
```
## Tips

### Using the Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
kubectl get secret ${CLUSTER_NAME}-admin-kubeconfig -o json | jq ".data.kubeconfig" -r | base64 -d > ${CLUSTER_NAME}.kubeconfig
export KUBECONFIG=${CLUSTER_NAME}.kubeconfig
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

### Troubleshooting HiveAdmission

To diagnose a hiveadmission failure, try running the operation directly against the registered hiveadmission API server.

For instance, try this:
```sh
# kubectl create --raw /apis/admission.hive.openshift.io/v1alpha1/dnszones -f config/samples/hiveadmission-review-failure.json -v 8 | jq
```

### Installing Federation

Ensure that you have the kubefed2 command installed:

```
go get -u github.com/kubernetes-sigs/federation-v2/cmd/kubefed2
```

Install federation components:

```
make install-federation
```

### Federation Example

An example etcd operator deployment is included in `contrib/federation_example`.

To deploy the example, run:

```
kubectl apply -f ./contrib/federation_example
```

The example artifacts install etcd-operator on a cluster with the label `etcdoperator: yes`

In order to automatically install the operator on a cluster  you create with Hive,
first create the cluster, then label its corresponding `federatedcluster` resource with
the appropriate label:

```
kubectl label federatedcluster/CLUSTER_NAME -n federation-system etcdoperator=yes
```

### Deploy using Minishift

The Hive controller and the operator can run on top of the OpenShift(version 3.11) provided by [Minishift](https://github.com/minishift/minishift).

Steps:

  - Start minishift
    ```
    $ minishift start
    ```
  - Login to the cluster as admin

    ```
    $ oc login -u system:admin
    ```

  - Give cluster-admin role to `admin` and `developer` user
    ```
    $ oc adm policy add-cluster-role-to-user cluster-admin system:admin
    $ oc adm policy add-cluster-role-to-user cluster-admin developer
    ```
  - Follow steps in [deployment Options](#deployment-options)
