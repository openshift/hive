# Installing Hive

## Prerequisites

* [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)
* [oc](https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/)

## Deployment Options

Hive contains an operator which is responsible for handling deployment logic for the rest of the components.

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

### Deploy using Minishift

The Hive controller and the operator can run on top of the OpenShift(version 3.11) provided by [Minishift](https://github.com/minishift/minishift).

Steps:

  - Start minishift
    ```bash
    $ minishift start
    ```bash
  - Login to the cluster as admin

    ```bash
    $ oc login -u system:admin
    ```

  - Give cluster-admin role to `admin` and `developer` user
    ```bash
    $ oc adm policy add-cluster-role-to-user cluster-admin developer
    $ oc adm policy add-cluster-role-to-user cluster-admin admin
    ```
  - Follow steps in [Deployment Options](#deployment-options)

