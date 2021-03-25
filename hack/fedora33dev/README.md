# Unrefined Fedora 33 Docker Build Steps

Workaround for an error you will see trying to do make docker-dev-push on Fedora 33 where glibc errors surface due to conflicts between the binaries you compiled on your system, and the OS in the Docker container where they run:

```
oc logs hive-operator-7dfdb6d59-vkttv -n hive
/opt/services/hive-operator: /lib64/libc.so.6: version `GLIBC_2.32' not found (required by /opt/services/hive-operator)
```

*Run all commands from the root of your hive.git.*

Build the base development image (really just one time needed):

```bash
$ docker build -t hive-dev-base -f hack/fedora33dev/Dockerfile.devbase .
```

Run script to build hive binaries locally (much faster than from scratch in a container), push to a registry, and restart all Hive pods in your current Kube context.

```bash
$ IMG="quay.io/username/hive:latest" hack/fedora33dev/docker-dev-push.sh
```


