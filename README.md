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
    - [Watch the ClusterDeployment](#watch-the-clusterdeployment)
    - [Delete your ClusterDeployment](#delete-your-clusterdeployment)
  - [Tips](#tips)
    - [Using the Admin Kubeconfig](#using-the-admin-kubeconfig)
    - [Troubleshooting Deprovision](#troubleshooting-deprovision)
    - [Troubleshooting HiveAdmission](#troubleshooting-hiveadmission)
    - [Deploy using Minishift](#deploy-using-minishift)
    - [Access the WebConsole](#access-the-webconsole)
  - [Documentation](#documentation)

# OpenShift Hive
API driven OpenShift cluster provisioning and management

## Prerequisites

* [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)
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

 1. `$ oc scale -n hive deployment.v1.apps/hive-operator --replicas=0`
 1. `$ make run-operator`

### Run Hive From Source

NOTE: assumes you have previously deployed using one of the above methods.

 1. `$ oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0`
 1. `$ make run`

## Using Hive

1. Obtain a pull secret from try.openshift.com and place in your home directory at `$HOME/.pull-secret`.
1. Ensure that you are logged in to AWS via the AWS CLI or that your AWS credentials are set in environment variables: AWS_ACCESS_KEY_ID & AWS_SECRET_ACCESS_KEY
1. Note the use of an SSH key below to access the instances if necessary. (this should typically not be required)

### Create a ClusterDeployment using the latest OpenShift release and installer image

Hiveutil offers a create-cluster subcommand which simplifies creating the API custom resources for a Hive ClusterDeployment.

```bash
make hiveutil
```


```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com mycluster
```

To view what create-cluster generates, *without* submitting it to the API server, add `-o yaml` to the above command. If you need to make any changes not supported by create-cluster options, the output can be saved, edited, and then submitted with `oc apply`.
By default this command assumes the latest Hive master CI build, and the latest OpenShift stable release. `--hive-image` can be specified to use a specific Hive image to run the install, and `--release-image` can be specified to control which OpenShift release image to install in the cluster.

### Watch the ClusterDeployment

* Get the current namespace in which the `oc process` command was executed
* Get the install pod name
  ```
  $ oc get pods -o json --selector job-name==${CLUSTER_NAME}-install | jq -r '.items | .[].metadata.name'
  ```
* Run following command to watch the cluster deployment
  ```
  $ oc logs -f <install-pod-name> -c hive
  ```
  Alternatively, you can watch the summarized output of the installer using
  ```
  $ oc exec -c hive <install-pod-name> -- tail -f /tmp/openshift-install-console.log
  ```

### Delete your ClusterDeployment

```bash
$ oc delete clusterdeployment ${CLUSTER_NAME} --wait=false
```
## Tips

### Using the Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
oc get secret ${CLUSTER_NAME}-admin-kubeconfig -o jsonpath='{ .data.kubeconfig }' | base64 -d > ${CLUSTER_NAME}.kubeconfig
export KUBECONFIG=${CLUSTER_NAME}.kubeconfig
oc get nodes
```

### Troubleshooting Deprovision

After deleting your cluster deployment you will see an uninstall job created. If for any reason this job gets stuck you can:

 1. Delete the uninstall job, it will be recreated and tried again.
 2. Manually delete the uninstall finalizer allowing the cluster deployment to be deleted, but note that this may leave artifacts in your AWS account.
 3. You can manually run the uninstall code with `hiveutil` to delete AWS resources based on their tags.
    * Run `make hiveutil`
    * Get your cluster tag i.e. `infraID` from the following command output.
      ```bash
      $ oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.infraID }'
      ```
    * In case your cluster deployment is not available, you can find the tag in AWS console on any object from that cluster.
    * Run following command to deprovision artifacts in the AWS.
      ```bash
      $ bin/hiveutil aws-tag-deprovision --loglevel=debug kubernetes.io/cluster/<infraID>=owned
      ```

### Troubleshooting HiveAdmission

To diagnose a hiveadmission failure, try running the operation directly against the registered hiveadmission API server.

For instance, try this:
```sh
# oc create --raw /apis/admission.hive.openshift.io/v1alpha1/dnszones -f config/samples/hiveadmission-review-failure.json -v 8 | jq
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
    $ oc adm policy add-cluster-role-to-user cluster-admin developer
    ```
  - Follow steps in [deployment Options](#deployment-options)

### Access the WebConsole

* Get the webconsole URL
  ```
  $ oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.webConsoleURL }'
  ```

* Retrive the password for `kubeadmin` user
  ```
  $ oc get secret ${CLUSTER_NAME}-admin-password -o jsonpath='{ .data.password }' | base64 -d
  ```

## Documentation

* [Developing Hive](./docs/developing.md)
* [SyncSet](./docs/syncset.md)
* [SyncIdentityProvider](./docs/syncidentityprovider.md)
