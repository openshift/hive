<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Developing Hive](#developing-hive)
  - [Prerequisites](#prerequisites)
  - [Build and run tests](#build-and-run-tests)
  - [Setting up the development environment](#setting-up-the-development-environment)
    - [Cloning the repository](#cloning-the-repository)
  - [Deploying with Kubernetes In Docker (kind)](#deploying-with-kubernetes-in-docker-kind)
  - [Writing/Testing Code](#writingtesting-code)
    - [Run Hive Operator](#run-hive-operator)
      - [Directly from source](#directly-from-source)
      - [Run Hive Operator Using Custom Images](#run-hive-operator-using-custom-images)
    - [Run Hive Controllers From Source](#run-hive-controllers-from-source)
  - [Developing Hiveutil Install Manager](#developing-hiveutil-install-manager)
  - [Enable Debug Logging In Hive Controllers](#enable-debug-logging-in-hive-controllers)
  - [Using Serving Certificates](#using-serving-certificates)
    - [Generating a Certificate](#generating-a-certificate)
    - [Using Generated Certificate](#using-generated-certificate)
  - [Dependency management](#dependency-management)
    - [Installing Dep](#installing-dep)
    - [Updating Dependencies](#updating-dependencies)
    - [Re-creating vendor Directory](#re-creating-vendor-directory)
    - [Running the e2e test locally](#running-the-e2e-test-locally)
    - [TIP](#tip)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Developing Hive

## Prerequisites

- Git
- Make
- A recent Go distribution (>=1.12)
- [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)

## Build and run tests

To build and test your local changes, run:

```bash
make
```

To only run the unit tests:

```bash
make test
```

## Setting up the development environment

### Cloning the repository

Get the sources from GitHub:

```bash
cd $GOPATH/src/openshift
git clone https://github.com/openshift/hive.git
```

## Deploying with Kubernetes In Docker (kind)

[Kind](https://github.com/kubernetes-sigs/kind) can be used as a lightweight development environment for deploying and testing Hive. The following instructions cover creating an insecure local registry (allowing for dramatically faster push/pull), and configuring your host OS, as well as the kind cluster to access it. This approch runs Hive in a container as you would in production, giving you the best coverage for manual testing.

This approach requires [Docker](https://docs.docker.com/install). At present we do not have kind working with podman.

Deploy a local insecure registry container, and configure your host docker daemon to be able to use it:

```bash
./hack/create-insecure-registry.sh
```

Create a kind cluster named 'hive' to deploy to. You can create as many kind clusters as you need.:

```bash
./hack/create-kind-cluster.sh hive
```

`docker ps` should now show you a "registry" and a "hive" container running.

```bash
$ docker ps
CONTAINER ID        IMAGE                  COMMAND                  CREATED             STATUS              PORTS                                  NAMES
2756e565065a        kindest/node:v1.15.3   "/usr/local/bin/entr…"   29 hours ago        Up 29 hours         40393/tcp, 127.0.0.1:40393->6443/tcp   hive-control-plane
1dc8a3c59d84        registry:2             "/entrypoint.sh /etc…"   2 weeks ago         Up 8 days           0.0.0.0:5000->5000/tcp                 registry
```

Configure kubectl/oc to talk to your new cluster:

```bash
export KUBECONFIG="$(kind get kubeconfig-path --name="hive")"
```

**NOTE:** If you do not have `cfssljson` and `cfssl` installed, run the following command to install, otherwise, ignore this.

```bash
go get -u github.com/cloudflare/cfssl/cmd/cfssljson
go get -u github.com/cloudflare/cfssl/cmd/cfssl
```

You can now build your local Hive source as a container, push to the local registry, and deploy Hive. Because we are not running on OpenShift we must also create a secret with certificates for the hiveadmission webhooks.

```bash
IMG=172.17.0.1:5000/hive:latest make docker-dev-push
DEPLOY_IMAGE=172.17.0.1:5000/hive:latest make deploy
./hack/hiveadmission-dev-cert.sh
```

Hive should now be running.

You can leave your registry container running indefinitely. The kind cluster can be replaced quickly as necessary:

```bash
kind delete cluster --name hive
./hack/create-kind-cluster.sh hive
```

## Adopting ClusterDeployments

It is possible to adopt cluster deployments into Hive, potentially even fake or kind clusters. This can be useful for developers who would like to work on functionality separate from actual provisioning.

To create a kind cluster and adopt:

```bash
./hack/create-kind-cluster.sh cluster1
bin/hiveutil create-cluster --base-domain=new-installer.openshift.com kind-cluster1 --adopt --adopt-admin-kubeconfig=$(kind get kubeconfig-path --name="cluster1") --adopt-infra-id=fakeinfra --adopt-cluster-id=fakeid
```

NOTE: when using a kind cluster not all controllers will be functioning properly as it is not an OpenShift cluster and thus lacks some of the CRDs our controllers use. (ClusterState, RemoteMachineSet, etc)

Alternatively you can use any valid kubeconfig for live or since deleted clusters.

Deprovision will run but find nothing to delete if no resources are tagged with your fake infrastructure ID.


## Writing/Testing Code

Our typical approach to manually testing code is to deploy Hive into your current cluster as defined by kubeconfig, scale down the relevant component you wish to test, and then run its code locally.

### Run Hive Operator

You can run the Hive operator using your source code using any one method from below

#### Directly from source

NOTE: assumes you have [previously deployed Hive](install.md)

```bash
oc scale -n hive deployment.v1.apps/hive-operator --replicas=0
make run-operator
```

#### Run Hive Operator Using Custom Images

 1. Build and publish a custom Hive image from your current working dir: `$ IMG=quay.io/{username}/hive:latest make buildah-dev-push`
 2. Deploy with your custom image: `$ DEPLOY_IMAGE=quay.io/{username}/hive:latest make deploy`
 3. After code changes you need to rebuild the Hive images as mentioned in step 1.
 4. Delete the running Hive pods using following command, so that the new pods will be running using the latest images built in the previous step.

```bash
oc delete pods --all -n hive
```

### Run Hive Controllers From Source

NOTE: assumes you have [previously deployed Hive](install.md)

```bash
oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0
make run
```

## Developing Hiveutil Install Manager

We use a hiveutil subcommand for the install-manager, in pods and thus in an image to wrap the openshift-install process and upload artifacts to Hive. Developing this is tricky because it requires a published image and ClusterImageSet. Instead, you can hack together an environment as follows:

 1. Create a ClusterDeployment, allow it to resolve the installer image, but before it can complete:
   1. Scale down the hive-controllers so they are no longer running: `$ oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0`
   2. Delete the install job: `$ oc delete job ${CLUSTER_NAME}-install`
 2. Make a temporary working directory in your hive checkout: `$ mkdir temp`
 3. Compile your hiveutil changes: `$ make hiveutil`
 4. Set your pull secret as an env var to match the pod: `$ export PULL_SECRET=$(cat ~/pull-secret)`
 5. Run: `/bin/hiveutil install-manager --work-dir ~/go/src/github.com/openshift/hive/temp --log-level=debug hive ${CLUSTER_NAME}`

## Enable Debug Logging In Hive Controllers

Scale down the Hive operator to zero

```bash
oc scale -n hive deployment.v1.apps/hive-operator --replicas=0
```

Edit the controller deployment to replace the `info` log-level to `debug`.

```bash
oc edit deployment/hive-controllers -n hive
```

```yaml
spec:
      containers:
      - command:
        - /opt/services/manager
        - --log-level
        - debug
```

## Using Serving Certificates

The hiveutil command includes a utility to generate Letsencrypt certificates for use with clusters you create in Hive.

Prerequisites:
* The `certbot` command must be available and in the path of your machine. You can install it by following the instructions at:
  [https://certbot.eff.org/docs/install.html](https://certbot.eff.org/docs/install.html)
* You must have credentials for AWS available in your command line, either by a configured `~/.aws/credentials` or environment variables (`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`).

### Generating a Certificate

1. Ensure that the `hiveutil` binary is available (`make hiveutil`)
2. Run: `hiveutil certificate create ${CLUSTER_NAME} --base-domain ${BASE_DOMAIN}`
   where CLUSTER_NAME is the name of your cluster and BASE_DOMAIN is the public DNS domain for your cluster (Defaults to `new-installer.openshift.com`)

### Using Generated Certificate

The output of the certificate creation command will indicate where the certificate was created. You can then use the `hiveutil create-cluster` command to
create a cluster that uses the certificate.

NOTE: The cluster name and domain used to create the certificate must match the name and base domain of the cluster you create.

Example:
`hiveutil create-cluster mycluster --serving-cert=$HOME/mycluster.crt --serving-cert-key=$HOME/mycluster.key`


## Dependency management

### Installing Dep

Before you can use Dep you need to download and install it from GitHub:

```
 go get github.com/golang/dep/cmd/dep
```

This will install the `dep` binary into *_$GOPATH/bin_*.

### Updating Dependencies

If your work requires a change to the dependencies, you need to update the Dep configuration.

* Edit *_Gopkg.toml_* to change the dependencies as needed.

* Run `make vendor` to fetch changed dependencies.

* Test that everything still compiles with changed files in place by running `make clean && make`.

**Refer dep documents for more information.**

* [dep document about updating dependencies](https://golang.github.io/dep/docs/daily-dep.html#updating-dependencies)

* [dep document about adding a dependency](https://golang.github.io/dep/docs/daily-dep.html#adding-a-new-dependency)

### Re-creating vendor Directory

If you delete *_vendor_* directory which contain the needed {project} dependencies.

To recreate *_vendor_* directory, you can run the following command:

```
make vendor
```

This command calls and runs Dep.
Alternatively, you can run the Dep command directly.

```
dep ensure -v
```

### TIP

* The Dep cache located under *_$GOPATH/pkg/dep_*.
* If you see any Dep errors during `make vendor`, you can remove local cached directory and try again.

## Running the e2e test locally

The e2e test deploys Hive on a cluster, tests that all Hive components are working properly, then creates a cluster
with Hive and ensures that Hive works properly with the installed cluster. It finally tears down the created cluster.

You can run the e2e test by pointing to your own cluster (via the `KUBECONFIG` environment variable).

Ensure that the following environment variables are set:

- `KUBECONFIG` - Must point to a valid Kubernetes configuration file that allows communicating with your cluster.
- `AWS_ACCESS_KEY_ID` - AWS access key for your AWS account
- `AWS_SECRET_ACCESS_KEY` - AWS secret access key for your AWS account

- `HIVE_IMAGE` - Hive image to deploy to the cluster
- `RELEASE_IMAGE` - OpenShift release image to use for the e2e test cluster
- `CLUSTER_NAMESPACE` - Namespace where clusterdeployment will be created for the e2e test
- `BASE_DOMAIN` - DNS domain to use for the test cluster (a corresponding Route53 public zone must exist on your account)
- `ARTIFACT_DIR` - Directory where logs will be placed by the e2e test
- `SSH_PUBLIC_KEY_FILE` - Path to a public ssh key to use for the test cluster
- `PULL_SECRET_FILE` - Path to file containing a pull secret for the test cluster

For example values for these variables, see `hack/local-e2e-test.sh`

Run the Hive e2e script:

`hack/e2e-test.sh`

## Viewing Metrics with Prometheus

Hive publishes a number of metrics that can be scraped by prometheus. If you do not have an in-cluster prometheus that can scrape hive's endpoint, you can deploy a stateless prometheus pod in the hive namespace with:

```
oc apply -f config/prometheus
oc port-forward svc/prometheus -n hive 9090:9090
```

Once the pods come up you should be able to view prometheus at http://localhost:9090.

Hive metrics have a hive_ or controller_runtime_ prefix.

Note that this prometheus uses an emptyDir volume and all data is lost on pod restart.
