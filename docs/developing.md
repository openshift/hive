# Developing Hive

## Prerequisites

- Git
- Make
- A recent Go distribution (>=1.11)
- [kustomize](https://github.com/kubernetes-sigs/kustomize#kustomize)

## Setting up the development environment

### Cloning the repository

Get the sources from GitHub:

```
$ cd $GOPATH/src/openshift
$ git clone https://github.com/openshift/hive.git
```

## Dependency management

### Installing Dep

Before you can use Dep you need to download and install it from GitHub:

```
$ go get github.com/golang/dep/cmd/dep
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
$ make vendor
```

This command calls and runs Dep.
Alternatively, you can run the Dep command directly.

```
$ dep ensure -v
```

### TIP

* The Dep cache located under *_$GOPATH/pkg/dep_*.
* If you see any Dep errors during `make vendor`, you can remove local cached directory and try again.

## Developing Hiveutil Install Manager

We use a hiveutil subcommand for the install-manager, in pods and thus in an image to wrap the openshift-install process and upload artifacts to Hive. Developing this is tricky because it requires a published image and ClusterImageSet. Instead, you can hack together an environment as follows:

 1. Create a ClusterDeployment, allow it to resolve the installer image, but before it can complete:
   1. Scale down the hive-controllers so they are no longer running: `kubectl scale -n hive deployment.v1.apps/hive-controllers --replicas=0`
   1. Delete the install job: `k delete job dgoodwin1-install`
 1. Make a temporary working directory in your hive checkout: `mkdir temp`
 1. Compile your hiveutil changes: `make hiveutil`
 1. Set your pull secret as an env var to match the pod: `export PULL_SECRET=$(cat ~/pull-secret)`
 1. ../bin/hiveutil install-manager --work-dir /home/dgoodwin/go/src/github.com/openshift/hive/temp --log-level=debug hive dgoodwin1
