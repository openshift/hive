# Central Machine Management

Central Machine Management (CMM) is a machine management pattern where machines are managed centrally from Hive via the [Cluster API](https://github.com/kubernetes-sigs/cluster-api) as opposed to from the managed cluster via the [Machine API](https://github.com/openshift/cluster-api). CMM can be enabled when there is reduced trust in managed clusters.

## Generate Custom Resource Definitions

Generate [Cluster API](https://github.com/kubernetes-sigs/cluster-api) CRDs by running:

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

The following patch can be used to append `AlphaMachineManagement` to the enabled list and set `featureSet: Custom`,

```
oc patch hiveconfig hive --type='json' -p='[{"op": "add", "path": "/spec/featureGates/custom/enabled/-", "value": "AlphaMachineManagment"},{"op": "replace", "path": "/spec/featureGates/featureSet", "value": "Custom"}]'
```

## Enable Central Machine Management for ClusterDeployment

CMM can be enabled for a `ClusterDeployment` by setting `cd.spec.machineManagement.central = {}`.

```
    spec:
      machineManagement:
        central: {}
```

Once CMM is enabled for a `ClusterDeployment`, a `targetNamespace` is generated and set as `cd.spec.machineManagement.targetNamespace`. Secrets necessary for actuating `MachineSets` are copied into the `ClusterDeployment`'s `targetNamespace`. Cluster API `MachineSets` and `MachineTemplates` are then reconciled within the `targetNamespace`.