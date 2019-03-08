# Deploying Hive via OLM

Hive can be deployed as an OLM operator. We do not currently publish artifacts, so to do so you need to build your own artifacts as follows.

Ensure the Hive namespace exists:

```
oc new-project hive
```

Identify the current version of the operator you wish to replace, if applicable. (TODO)

```
GIT_HASH=`git rev-parse --short HEAD`
GIT_COMMIT_COUNT=`git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count`
./hack/generate-operator-bundle.py bundle/ 0.1.5-4b53b492 $GIT_COMMIT_COUNT $GIT_HASH quay.io/dgoodwin/hive:latest alpha
```

Build a registry image containing your bundle and publish to quay:

```
sudo buildah bud --file build/olm-registry/Dockerfile --tag "quay.io/dgoodwin/hive-registry:latest" .
sudo podman push quay.io/dgoodwin/hive-registry:latest
```

Create a CatlogSource, OperatorGroup, and Subscription for hive:


Create a CatalogSource in your cluster where OLM is running. This will spawn a registry pod using your image just published.

```
kubectl apply -f hack/olm-registry/
```

Wait a few minutes and you should see a ClusterServiceVersion in the 'hive' namespace, then an operator pod, and soon the rest of hive.
