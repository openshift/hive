<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Prerequisites](#prerequisites)
  - [External tools](#external-tools)
- [Build and run tests](#build-and-run-tests)
- [Setting up the development environment](#setting-up-the-development-environment)
  - [Cloning the repository](#cloning-the-repository)
- [Obtaining a Cluster](#obtaining-a-cluster)
  - [Creating a Kubernetes In Docker (kind) Cluster](#creating-a-kubernetes-in-docker-kind-cluster)
- [Deploying from Source](#deploying-from-source)
  - [Full Container Build](#full-container-build)
  - [Using public images](#using-public-images)
  - [Running Code Locally](#running-code-locally)
    - [hive-operator](#hive-operator)
  - [hive-controllers](#hive-controllers)
- [Developing Hiveutil Install Manager](#developing-hiveutil-install-manager)
- [Enable Debug Logging In Hive Controllers](#enable-debug-logging-in-hive-controllers)
- [Using Serving Certificates](#using-serving-certificates)
  - [Generating a Certificate](#generating-a-certificate)
  - [Using Generated Certificate](#using-generated-certificate)
- [Code editors and multi-module repositories](#code-editors-and-multi-module-repositories)
- [Updating Hive APIs](#updating-hive-apis)
- [Importing Hive APIs](#importing-hive-apis)
- [Dependency management](#dependency-management)
  - [Updating Dependencies](#updating-dependencies)
  - [Re-creating vendor Directory](#re-creating-vendor-directory)
  - [Updating the Kubernetes dependencies](#updating-the-kubernetes-dependencies)
  - [Vendoring the OpenShift Installer](#vendoring-the-openshift-installer)
- [Running the e2e test locally](#running-the-e2e-test-locally)
- [Viewing Metrics with Prometheus](#viewing-metrics-with-prometheus)
- [Hive Controllers Profiling](#hive-controllers-profiling)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Developing Hive

## Prerequisites

- Git
- Make
- A recent Go distribution (>=1.23)

### External tools

- [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)
- [imagebuilder](https://github.com/openshift/imagebuilder)
- [mockgen](https://github.com/golang/mock)

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
git clone https://github.com/openshift/hive.git
```

## Obtaining a Cluster

A Kubernetes or OpenShift cluster is required to run Hive from source. If you would prefer to work locally, it is possible to use kind.

### Creating a Kubernetes In Docker (kind) Cluster

[Kind](https://github.com/kubernetes-sigs/kind) can be used as a lightweight development environment for deploying and testing Hive. Currently, we support 0.8.1 version of Kind. The following instructions cover creating an insecure local registry (allowing for dramatically faster push/pull), and configuring your host OS, as well as the kind cluster to access it. This approch runs Hive in a container as you would in production, giving you the best coverage for manual testing.

This approach requires either [Podman](https://podman.io/) or [Docker](https://docs.docker.com/install).

If you encounter ImagePullErrors, and your kind container cannot reach your registry container, you may be experiencing problems with Fedora (at least 32, possibly earlier as well) and Docker. You can attempt to work around this by changing the FirewallBackend in the /etc/firewalld/firewalld.conf file from nftables to iptables and restarting docker. (see [this comment](https://github.com/kubernetes-sigs/kind/issues/1547#issuecomment-623756313)


Create a local insecure registry (if one does not already exist) and then a kind cluster named 'hive' to deploy to. You can create additional clusters if desired by providing a different name argument to the script.

```bash
./hack/create-kind-cluster.sh hive
export KUBECONFIG=~/.kube/kind-hive.kubeconfig
```

`podman ps` or `docker ps` should now show you a "registry" and a "hive" container running.

```bash
$ podman ps
CONTAINER ID        IMAGE                  COMMAND                  CREATED             STATUS              PORTS                       NAMES
ffddd07cd9e1        kindest/node:v1.18.2   "/usr/local/bin/entr…"   32 minutes ago      Up 32 minutes       127.0.0.1:36933->6443/tcp   hive-control-plane
8060ab7c8116        registry:2             "/entrypoint.sh /etc…"   2 days ago          Up 2 days           0.0.0.0:5000->5000/tcp      kind-registry
```

You should now have a `kind-hive` context in your kubeconfig and set to current.

**NOTE:** If you do not have `cfssljson` and `cfssl` installed, run the following command to install, otherwise, ignore this.

```bash
go install github.com/cloudflare/cfssl/cmd/cfssljson
go install github.com/cloudflare/cfssl/cmd/cfssl
```

You can now build your local Hive source as a container and push to the local registry using the build instructions below and:

```bash
export IMG=localhost:5000/hive:latest
```

You can leave your registry container running indefinitely. The kind cluster can be replaced quickly as necessary:

```bash
kind delete cluster --name hive
./hack/create-kind-cluster.sh hive
```

## Deploying from Source

### Full Container Build

The most resilient method of deploying Hive is to build and publish a container from scratch, and deploy manifests to the cluster currently referenced by your kubeconfig.

This method is quite slow, but reliable.

Podman:
```bash
export IMG="quay.io/{username}/hive:latest"
make buildah-dev-push deploy
oc delete pods -n hive --all
```

Docker:
```bash
export IMG="quay.io/{username}/hive:latest"
make image-hive docker-push deploy
oc delete pods -n hive --all
```

NOTE: If you are running on Kubernetes or kind, (not OpenShift), you will also need to create certificates for hiveadmission after running make deploy for the first time:

```bash
./hack/hiveadmission-dev-cert.sh
```

NOTE: If you are running on Kubernetes or kind >= version 1.24, (not OpenShift), you will also need to create secrets for serviceaccounts after running make deploy for the first time:

```bash
./hack/create-service-account-secrets.sh
```

### Using public images

If you cannot login to registry.ci.openshift.org, a temporary solution is to use
public images during build and test. At the time of writing, the following public images
do the trick.

```shell
export EL8_BUILD_IMAGE=registry.ci.openshift.org/openshift/release:golang-1.23
export EL9_BUILD_IMAGE=registry.ci.openshift.org/openshift/release:golang-1.23
export BASE_IMAGE=registry.ci.openshift.org/origin/4.16:base
# NOTE: This produces images which are not FIPS-compliant.
export GO="CGO_ENABLED=0 go"
```

You can now build the development image with the hiveutil binaries from your local system:

```bash
export IMG="quay.io/{username}/hive:latest}"
make build image-hive docker-push deploy
oc delete pods -n hive --all
```

NOTE: If you are running on Kubernetes or kind, (not OpenShift), you will also need to create certificates for hiveadmission after running make deploy for the first time:

```bash
./hack/hiveadmission-dev-cert.sh
```

### Running Code Locally

Our typical approach to manually testing code is to deploy Hive into your current cluster as defined by kubeconfig, scale down the relevant component you wish to test, and then run its code locally.
It is also possible to run the controllers locally on the host OS using your current kubeconfig context. To do this you would deploy normally per above, scale down the appropriate Deployment for the component you wish to work on, and then run the code locally.

TODO: this section needs updating for breakout of clustersync controllers, and various configurations required for controllers.

#### hive-operator

```bash
oc scale -n hive deployment.v1.apps/hive-operator --replicas=0
make run-operator
```

### hive-controllers

```bash
oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0
HIVE_NS="hive" make run
```

Kind users should also specify `HIVE_IMAGE="localhost:5000/hive:latest"` as the default image location cannot be authenticated to from Kind clusters, resulting in inability to launch install pods.

## Developing Hiveutil Install Manager

We use a hiveutil subcommand for the install-manager, in pods and thus in an image to wrap the openshift-install process and upload artifacts to Hive. Developing this is tricky because it requires a published image and ClusterImageSet. Instead, you can hack together an environment as follows:

 1. Create a ClusterDeployment, allow it to resolve the installer image, but before it can complete:
   1. Scale down the hive-controllers so they are no longer running: `$ oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0`
   2. Delete the install job: `$ oc delete job ${CLUSTER_NAME}-install`
 2. Make a temporary working directory in your hive checkout: `$ mkdir temp`
 3. Compile your hiveutil changes: `$ make build`
 4. Set your pull secret as an env var to match the pod: `$ export PULL_SECRET=$(cat ~/pull-secret)`
 5. Run: `/bin/hiveutil install-manager --work-dir $GOPATH/src/github.com/openshift/hive/temp --log-level=debug hive ${CLUSTER_NAME}`

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
3. Follow the instructions in the output of the hiveutil certificate command. You will need to setup the letsencrypt CA as trusted one time only for your cluster.

### Using Generated Certificate

The output of the certificate creation command will indicate where the certificate was created. You can then use the `hiveutil create-cluster` command to
create a cluster that uses the certificate.

NOTE: The cluster name and domain used to create the certificate must match the name and base domain of the cluster you create.

Example:
`hiveutil create-cluster mycluster --serving-cert=$HOME/mycluster.crt --serving-cert-key=$HOME/mycluster.key`

## Code editors and multi-module repositories

Hive is a [multi-module repository](https://github.com/golang/go/wiki/Modules#faqs--multi-module-repositories) with
the various submodules.

`gopls` currently doesn't officially support multi-module repositories. There are current recommendations for some
code editors to work around the current issue.

See [gopls doc](https://github.com/golang/tools/blob/master/gopls/doc/workspace.md#multiple-modules) for next steps.

## Updating Hive APIs

Hive is a [multi-module repository](https://github.com/golang/go/wiki/Modules#faqs--multi-module-repositories) with
the `github.com/openshift/hive/apis` go submodule.

A separate `github.com/openshift/hive/apis` submodule allows other repositories to easily import the Hive APIs without
having to deal with the complex vendoring introduced by dependencies of Hive controllers like the OpenShift Installer.

Since Hive's `github.com/openshift/hive` and `root` modules both use vendor directories for all their dependencies, **ANY**
change in the `github.com/openshift/hive/apis` module requires updating the corresponding dependency in the `root` module for the change to
take effect.

Therefore, these steps must be followed when updating the Hive APIs:

1. Make necessary modifications to `github.com/openshift/hive/apis`.

2. Follow the steps defined in [Updating Dependencies](#Updating-Dependencies) to update the dependecies of the `root` module

3. Now you can use various `make` targets or `go test` for testing your changes.

**IMPORTANT**: go versions => 1.15 default to `-mod=vendor` for all the subcommands and therefore will
only use the copy of the modules in the `vendor/` directory. Any time you make changes to the `apis` submodule and do
not update the dependencies of the root module, all the builds and tests will continue to use the old version of the
submodule.

## Importing Hive APIs

External projects that want to import the Hive APIs can import `github.com/openshift/hive/apis` into their go.mod.
The module uses the same versioning as the root Hive module.

```
go get -u github.com/openshift/hive/apis@{required version}
```

For getting the latest Hive APIS,

```
go get -u github.com/openshift/hive/apis@master
```

## Dependency management

### Updating Dependencies

If your work requires a change to dependencies, you need to update the vendored modules.

* If you are upgrading an existing dependency, run `go get [module]`. If you are adding a dependency, then you should
not need to do anything explicit for this step. The go tooling should pick up the dependency from the `import` directives
that you added in code.

* Run `make vendor` to fetch changed dependencies.

* Test that everything still compiles with changed files in place by running `make`.

**Refer to Go modules documents for more information.**

* [go modules wiki](https://github.com/golang/go/wiki/Modules)

### Re-creating vendor Directory

If you delete the `vendor` directory which contains the necessary project dependencies, you can recreate it via the following command:

```
make vendor
```

### Updating the Kubernetes dependencies

Hive is a [multi-module repository](https://github.com/golang/go/wiki/Modules#faqs--multi-module-repositories) with a `github.com/openshift/hive/apis` submodule.

This submodule defines its own dependencies, most commonly from the Kubernetes ecosystem. All build artifacts are
generated from the `root` module and therefore the version of all dependencies used is defined by the `go.mod` in the
root module, but making sure the dependecies of the submodule match those of the root module helps reduce divergence
and dependency pain for consumers.

So whenever the root module updates the Kubernetes ecosystem dependencies,

1. Update the submodules to use the desired version of the Kubernetes ecosystem.

2. Update the root module dependencies to the desired version of the Kubernetes ecosystem.

You can validate that the root and submodule dependencies match by running

```
make modcheck
```

This will exit nonzero if any discrepancies are found, and the output will denote which dependencies
are mismatched. For example:
```
$ make modcheck
go run ./hack/modcheck.go
XX require github.com/go-logr/logr: root(v1.2.2) apis(v1.2.0)
XX require github.com/google/gofuzz: root(v1.2.0) apis(v1.1.0)
exit status 1
make: *** [Makefile:325: modcheck] Error 1
```
In this example, address the first error by updating the version of the `require github.com/go-logr/logr` dependency in `apis/go.mod` from `v1.2.0` to `v1.2.2`.
Repeat for the other errors until the output is clean.
Don't forget to `make vendor` when you're done to sync the vendor directories with your go.mod changes.

### Vendoring the OpenShift Installer

For various reasons, Hive vendors the OpenShift Installer. The OpenShift installer brings in quite a few dependencies, so it is important to know the general flow of how to vendor the latest version of the OpenShift Installer.

Things to note:
* Hive vendors the installer from `@master`, NOT from `@latest`. For go modules, `@latest` means the latest git tag, which for the installer is not up to date.
* The `go.mod` file contains a section called `replace`. The purpose of this section is to force go modules to use a specific version of that dependency.
* `replace` directives may need to be copied from the OpenShift Installer or possibly other Hive dependencies. In other words, any dependency may need to be pinned to a specific version.
* `go mod tidy` is used to add (download) missing dependencies and to remove unused modules.
* `go mod vendor` is used to copy dependent modules to the vendor directory.


The following is a basic flow for vendoring the latest OpenShift Installer. Updating Go modules can sometimes be complex, so it is likely that the flow below will not encompass everything needed to vendor the latest OpenShift Installer. If more steps are needed, please document them here so that the Hive team will know other possible things that need to be done.

Basic flow for vendoring the latest OpenShift Installer:

* Compare the `replace` section of the OpenShift Installer `go.mod` with the `replace` section of the Hive `go.mod`. If the Hive `replace` section has the same module listed as the OpenShift Installer `replace` section, ensure that the Hive version matches the installer version. If it doesn't, change the Hive version to match.

* Edit `go.mod` and change the OpenShift Installer `require` to reference `master` (or a specific commit hash) instead of the last version. Go will change it to the version that master points to, so this is a temporary change.
```
github.com/openshift/installer master
```

* Run `make vendor`. This make target runs both `go mod tidy` and `go mod vendor` which get the latest modules, cleanup unused modules and copy the moduels into the Hive git tree.
```
make vendor
```

* If `go mod tidy` errors with a message like the following, then check Hive's usage of that package. In this case, the Hive import is importing an old version of the API. It needs to instead import v1beta1. Fix the hive code and re-run `go mod tidy`. This may need to be done multiple times.
```
github.com/openshift/hive/pkg/controller/machinepool imports
	github.com/openshift/machine-api-operator/pkg/apis/vsphereprovider/v1alpha1: module github.com/openshift/machine-api-operator@latest found (v0.2.0), but does not contain package github.com/openshift/machine-api-operator/pkg/apis/vsphereprovider/v1alpha1
```

* If `go mod tidy` errors with a message like the following, then check the installer's replace directives for that go module so that Hive is pulling in the same version. Re-run the `go mod tidy` once the replace directive has been added or updated. This process may need to be followed several times to clean up all of the errors.
```
github.com/openshift/hive/pkg/controller/machinepool imports
	github.com/openshift/installer/pkg/asset/machines/aws imports
	sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1: module sigs.k8s.io/cluster-api-provider-aws@latest found (v0.5.3, replaced by github.com/openshift/cluster-api-provider-aws@v0.2.1-0.20200316201703-923caeb1d0d8), but does not contain package sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1
```

* If `go mod tidy` errors with a message like the following, then check the installer's replace directives for that go module so that Hive is pulling in the same version. Re-run the `go mod tidy` once the replace directive has been added or updated. This process may need to be followed several times to clean up all of the errors.
```
go: sigs.k8s.io/cluster-api-provider-azure@v0.0.0: reading sigs.k8s.io/cluster-api-provider-azure/go.mod at revision v0.0.0: unknown revision v0.0.0
```

* If `go mod tidy` errors with a message like the following, then check the installer's replace directives to see if the replace needs to be updated. In this specific case, the replace was correct, but Hive is referring to `awsproviderconfig/v1beta1`, and the module has renamed that directory to `awsprovider/v1beta1`. Fix the Hive code and re-run `go mod tidy`
```
github.com/openshift/hive/cmd/manager imports
	sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1: module sigs.k8s.io/cluster-api-provider-aws@latest found (v0.5.3, replaced by github.com/openshift/cluster-api-provider-aws@v0.2.1-0.20200506073438-9d49428ff837), but does not contain package sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1

```

* Once `go mod vendor` succeeds, run `make` to ensure everything builds and test correctly:
```
make
```
* If `make` errors, that may mean that Hive code needs to be updated to be compatible with the latest vendored code. Fix the Hive code and re-run `make`


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
oc apply -f config/prometheus/prometheus-configmap.yaml
oc apply -f config/prometheus/prometheus-deployment.yaml
oc port-forward svc/prometheus -n hive 9090:9090
```

Once the pods come up you should be able to view prometheus at http://localhost:9090.

Hive metrics have a hive_ or controller_runtime_ prefix.

Note that this prometheus uses an emptyDir volume and all data is lost on pod restart. You can instead use the deployment yaml with pvc if desired:

```
oc apply -f config/prometheus/prometheus-deployment-with-pvc.yaml
```

## Hive Controllers Profiling

[Profiling](https://go.dev/doc/diagnostics#profiling) is enabled by default for hive-controllers and hive-clustersync at port 6060.

Port 6060 is already exposed in the Service for both of these controllers.
You can forward it locally and the Service will pick a replica for you:

```bash
$ oc port-forward svc/hive-controllers -n hive 6060
```

If you wish to profile a specific replica, you can forward just that pod:

```bash
$ oc port-forward pod/hive-clustersync-0 6060
```

If you wish to profile multiple pods at once, you will need to choose a different local port for each:

```bash
$ oc port-forward pod/hive-clustersync-0 6060:6060
$ oc port-forward pod/hive-clustersync-1 6061:6060
```

See `oc port-forward --help` for more options.

Visit the web UI (e.g. http://localhost:6060/debug/pprof/) to view available profiles and some live data.

Grab a profile snapshot with curl:

```bash
$ curl "http://127.0.0.1:6060/debug/pprof/profile?seconds=300" > cpu.pprof
```

Display some text data on the snapshot:

```bash
$ go tool pprof --text cpu.pprof
```
