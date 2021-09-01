# Central Machine Management

## Generate Custom Resource Definitions

Generate CAPI CRDs by running:

```
controller-gen crd paths=./vendor/sigs.k8s.io/cluster-api/... paths=./thirdparty/... output:crd:dir=./hack/cluster-api/
```

## Deploy Custom Resource Definitions

CAPI CRDs are not deployed by default and must be applied manually to the Hive cluster.

```
oc apply -f ./hack/cluster-api/
```

## Alpha Feature Gate

The `AlphaMachineManagement` feature gate must be enabled in `hiveconfig` before the API will allow `cd.spec.machineManagement.central = {}` to be set for a `ClusterDeployment`.

```
  spec:
    featureGates:
      custom:
        enabled:
        - AlphaMachineManagement
      featureSet: Custom
```

```
oc patch hiveconfig hive --patch '{"spec":{"featureGates":{"custom":{"enabled":["AlphaMachineManagement"]},"featureSet":"Custom"}}}' --type=merge
```

## Request Central Machine Management for ClusterDeployment

CMM can be enabled for a ClusterDeployment by setting `cd.spec.machineManagement.central = {}`

```
    spec:
      machineManagement:
        central: {}
```

Once CMM is enabled, a `targetNamespace` is generated per `ClusterDeployment` and set as `cd.spec.machineManagement.targetNamespace`. Secrets necessary for creating CAPI `MachineSets` are copied into this `targetNamespace`. Cluster API `MachineSets` and `MachineTemplates` are created in the `targetNamespace`.