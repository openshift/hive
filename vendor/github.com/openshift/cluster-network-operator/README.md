# Cluster Network Operator

This is an operator that manages the networking components for an openshift cluster.


## Building

You can build an image using podman with

```
./hack/build-image.sh
```

You might need sudo:
```
BUILDCMD="sudo podman build" ./hack/build-image.sh
```

Or you could use a docker that supports multi-stage builds
```
BUILDCMD="docker build" ./hack/build-image.sh
```

## Running

There are some premade Kubernetes manifests to run a development build. After setting the image URL to something sane in the daemonset, do:

```
oc create -f manifests/
oc create -f sample-config.yaml
```

Then you can watch the daemonset with `kubectl -n openshift-cluster-network-operator logs <PODID>`.

## Example configuration

```yaml
apiVersion: "networkoperator.openshift.io/v1"
kind: "NetworkConfig"
metadata:
  name: "default"
  namespace: cluster-network-operator
spec:
  serviceNetwork: "172.30.0.0/16"
  clusterNetworks:
    - cidr: "10.128.0.0/14"
      hostSubnetLength: 9
  defaultNetwork:
    type: OpenshiftSDN
    openshiftSDNConfig:
      mode: Networkpolicy
```
