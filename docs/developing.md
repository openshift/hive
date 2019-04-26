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

