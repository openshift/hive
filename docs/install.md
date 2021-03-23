# Installing Hive

## Installing Community Release via OperatorHub

Hive is published to [OperatorHub](https://operatorhub.io/operator/hive-operator) weekly and this is the best method to install and use Hive if you do not need to build from source.

  1. Create a 'hive' namespace.
  1. In OpenShift web console navigate to Administrator perspective > Operators > OperatorHub.
  1. Search for “hive” and select the OpenShift Hive operator.
     1. Select the “alpha” update channel, install to a specific namespace, select the “hive” namespace just created, approval strategy automatic, and press Install.
     1. You should now have a hive-operator pod running in the hive namespace.
  1. Create a HiveConfig to trigger the the actual deployment of Hive. (can be done via the web UI with a couple clicks, or with oc apply)

```yaml
apiVersion: hive.openshift.io/v1
kind: HiveConfig
metadata:
  name: hive
spec:
 logLevel: debug
 targetNamespace: hive
```

The hive-operator pod should now deploy the remaining components (hive-controllers, hive-clustersync, hiveadmission), and once running Hive is now ready to begin accepting ClusterDeployments.

## Installing from Source

### Prerequisites

* [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)
* [oc](https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/)
* [go (version 1.13)](https://github.com/golang/go)

### Deployment Options

Hive contains an operator which is responsible for handling deployment logic for the rest of the components.

#### Deploy Hive Operator Using Latest Master Images

To deploy the operator from a git checkout:

  `$ make deploy`

By default the operator will use the latest images published by CI from the master branch.

You should now see hive-operator, hive-controllers, and hiveadmission pods running in the hive namespace.

#### Deploy Hive Operator Using Custom Images

 1. Build and publish a custom Hive image from your current working dir: `$ IMG=quay.io/dgoodwin/hive:latest make buildah-push`
 2. Deploy with your custom image: `$ DEPLOY_IMAGE=quay.io/dgoodwin/hive:latest make deploy`

#### Deploy Hive via OLM

While a community Hive operator bundle is now published weekly to OperatorHub (see above), in rare cases developers may want to build a bundle from source.  You can run or work off the test script below to generate a ClusterServiceVersion, OLM bundle+package, registry image, catalog source, and subscription. (WARNING: this is seldom used and may not always be working)

`$ REGISTRY_IMG="quay.io/dgoodwin/hive-registry" DEPLOY_IMG="quay.io/dgoodwin/hive:latest" hack/olm-registry-deploy.sh`

##### Verify that Hive is running
Run: `$ oc get pods -n hive`

Sample output:

```bash
$ oc get pods -n hive
NAME                                READY     STATUS    RESTARTS   AGE
hive-controllers-777bcb5b4d-nqw7w   1/1       Running   0          38m
hive-operator-57dc6446df-4wqnd      1/1       Running   0          38m
hiveadmission-5dfff7f575-4kcc4      1/1       Running   0          38m
hiveadmission-5dfff7f575-cqxgg      1/1       Running   0          38m
```

## Next Step

Provision a OpenShift cluster using Hive.
For details refer [using Hive](./using-hive.md) documentation.

