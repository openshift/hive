# Installing Hive

## Installing Community Release via OperatorHub

Hive is published to [OperatorHub](https://operatorhub.io/operator/hive-operator) weekly and this is the best method to install and use Hive if you do not need to build from source.

1. Create a `hive` namespace,
    ```
    $ oc new-project hive
    ```
1. Install the Hive Operator:
    * In the [OpenShift web console](https://docs.openshift.com/container-platform/latest/web_console/web-console-overview.html), navigate to [Administrator perspective](https://docs.openshift.com/container-platform/latest/web_console/web-console-overview.html#accessing-the-administrator-perspective_web-console-overview) > Operators > [OperatorHub](https://docs.openshift.com/container-platform/latest/operators/understanding/olm-understanding-operatorhub.html#olm-operatorhub-overview_olm-understanding-operatorhub).
    * Search for “hive” and select the "Hive for Red Hat OpenShift" operator and click Install.
    * Select the “alpha” update channel, install to a specific namespace (select the “hive” namespace previously created), approval strategy: automatic, and press Install.
    * You should now have a hive-operator pod running in the hive namespace.
1. Create a `HiveConfig` to trigger the actual deployment of Hive.
    * Create a `hive_config.yaml` file with the following content:

      ```yaml
      apiVersion: hive.openshift.io/v1
      kind: HiveConfig
      metadata:
        name: hive
      spec:
        logLevel: debug
        targetNamespace: hive
      ```
    * Apply `hive_config.yaml`,
      ```
      $ oc apply -f hive_config.yaml
      ```

The hive-operator pod should now deploy the remaining components (hive-controllers, hive-clustersync, hiveadmission), and once running Hive is now ready to begin accepting ClusterDeployments.

## Deploy From Source

See [developer instructions](developing.md)

# Verify that Hive is running

Run: `$ oc get pods -n hive`

Sample output:

```bash
$ oc get pods -n hive
hive-clustersync-0                  1/1     Running   0          34s
hive-controllers-5d67988cc8-97r5p   1/1     Running   0          35s
hive-machinepool-0                  1/1     Running   0          34s
hive-operator-5c7fdd6df8-jrxvt      1/1     Running   0          3m30s
hiveadmission-5bf565bd7-nqq9h       1/1     Running   0          32s
hiveadmission-5bf565bd7-tkf4c       1/1     Running   0          32s
```

# Next Step

Provision an OpenShift cluster using Hive.
For details refer [using Hive](./using-hive.md) documentation.

