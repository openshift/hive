# Unified Configuration of Hive controllers

## Summary

Each Hive controller loads its configuration from a ConfigDir. The ConfigDir provides
the controller a YAML serialized HiveConfig and all needed ConfigMaps and Secrets.
Hive operator ensures the ConfigDir is setup with latest copy of HiveConfig and
mounts all ConfigMaps and Secrets referenced in HiveConfig to ConfigDir so that the
controller can read the contents from disk.

## Motivation

Currently each Hive controller decides how it accepts configuration like using Env vars,
files on disk, k8s objects like Secrets fetched at runtime, or combinations of these.
But the source of all these configuration is one object HiveConfig, and the Hive operator
has the code to transform the HiveConfig contents to different destinations as expected
by these Hive controllers. There is a lot of new code required in Hive operator and
controller every time a new controller is added or new configuration is required. Also
there is no standard way to debug the configuration because of this heterogeneity.

To make sure it is easier to add new configuration, we must standardize the configuration
to one way for all controllers.

### Goals

1. No additional code required to support new configuration that only depends on HiveConfig
    content.
2. Minimal code required to support new configuration that depends on data from
    additional k8s objects like Secrets or ConfigMaps.
3. Reduce different types of configuration sources.
4. Remove decision making of how to configure the controllers.

### Non-Goals

## Proposal

All hive controllers accept a directory (ConfigDir) for configuration. This directory
provides,

1. YAML serialized HiveConfig,
2. Secrets specified in HiveConfig, and
3. ConfigMaps specified in HiveConfig.

```
$CONFIG_DIR/
    hiveconfig/
        config.yaml
    secrets/
        <secret-1>/
            <secret-1-key1>
            <secret-1-key2>
        <secret-2>/
            <secret-2-key1>
        ...
    configmaps/
        <configmap-1>/
            <configmap-1-key1>
        ...
```

All the Secrets or ConfigMaps that are specified in the HiveConfig by `LocalObjectReference`
are mounted to `$CONFIG_DIR/secrets|configmaps/<ref.name>` directory. When controllers
need access to certain Secret/ConfigMap as specified in the HiveConfig, it loads
the data from directory by constructing the filepath using the `$ref.name` and necessary
keys. The controllers do not require access to the k8s cluster where they are running
to load the Secrets or ConfigMaps.

### User Stories

#### Adding a new configuration which only requires HiveConfig data

```yaml
# lets assume a new field is added to hiveconfig for controllerA
spec:
  controllerAConfig:
    replicas: (integer)
```

Since this configuration does not depend on additional k8s objects, the controller can directly
use the new field by loading HiveConfig using `controllerutils.ReadHiveConfigFile`

#### Adding a new configuration that requires Secret

```yaml
# lets assume a new field is added to hiveconfig for controllerA
spec:
  controllerAConfig:
    secretRef:
      # this is secret in the TargetNamespace of HiveConfig.
      name: (string)
```

Since this configuration depends on secret, the controller cannot directly use the contents of the
secret. It needs to add `secretCollector` for this secret to Hive operator.

```go
func init() {
    secretCollectors = append(selectCollectors, controllerAConfigSecretCollector)
}

func controllerAConfigSecretCollector(instance *hivev1.HiveConfig) (secrets []corev1.LocalObjectReference) {
    if instance.ControllerAConfig != nil && instance.ControllerAConfig.SecretRef.Name != "" {
        secrets = append(secrets, instance.ControllerAConfig.SecretRef)
    }
    return secrets
}
```

Now the controller can read the Secret using `controllerutils.LoadSecret(reconciler.controllerAConfig.SecretRef.Name)`.

### Running Hive controllers locally

For developing the best path for running controller code is to build the controller binary and run is
locally and point it to the k8s cluster. Using a ConfigDir makes that easier.

Here's how you can setup ConfigDir locally,

1. Setup the HiveConfig

    ```sh
    $ mkdir -p $CONFIG_DIR/hiveconfig
    $ oc get hiveconfig <> -oyaml > $CONFIG_DIR/hiveconfig/config.yaml
    ```

2. Setup the necessary secrets/configmaps for the controller(s) that you need testing.

    For every secret that you want the local controller to have access to,

    ```sh
    $ mkdir-p $CONFIG_DIR/secrets/<secret name>
    $ oc -n hive extract secret/<secret name> --to $CONFIG_DIR/secrets/<secret name> --confirm
    ```

    For every configmap that you want the local controller to have access to,

    ```sh
    $ mkdir-p $CONFIG_DIR/configmap/<configmap name>
    $ oc -n hive extract configmap/<configmap name> --to $CONFIG_DIR/configmaps/<configmap name> --confirm
    ```

3. Now run the controller binary locally pointing it to the cluster using KUBECONFIG env var.

### Implementation Details

#### Mounting all the needed Secrets/ConfigMaps

The Hive operator needs to know which ConfigMaps/Secrets need to be mounted into the
controller pods. There needs to be code that given an instance of HiveConfig return the
Secrets and ConfigMaps that need to be mounted.

```go
// given an instance of HiveConfig return all the secrets that need to be mounted.
type secretCollector func (instance *hivev1.HiveConfig) []corev1.LocalObjectReference

// given an instance of HiveConfig return all the configmaps that need to be mounted.
type configMapCollector func (instance *hivev1.HiveConfig) []corev1.LocalObjectReference
```

Now define single responsibility collector functions for one type of controller/configuration.
The operator can store a static list of these collectors and iterator over them to generate a
list of objects that need to be mounted.

```go
// for every secret reference that are collected create a secret volume.
volumes = append(volumes, corev1.Volume{
    Name: fmt.Sprintf("secret_%s", ref.Name),
    VolumeSource: corev1.Volume{
        Secret: &corev1.SecretVolumeSource{
            SecretName: ref.Name,
        },
    },
})
volumesMounts = append(voumeMounts, corev1.VolumeMount{
    Name: fmt.Sprintf("secret_%s", ref.Name),
    Readonly: true,
    MountPath: filepath.Join(configDir, "secrets", ref.Name),
})
```

```go
// for every configmap reference that are collected create a secret volume.
volumes = append(volumes, corev1.Volume{
    Name: fmt.Sprintf("configmap_%s", ref.Name),
    VolumeSource: corev1.Volume{
        ConfigMap: &corev1.ConfigMapVolumeSource{
            LocalObjectReference: ref,
        },
    },
})
volumesMounts = append(voumeMounts, corev1.VolumeMount{
     Name: fmt.Sprintf("configmap_%s", ref.Name),
    Readonly: true,
    MountPath: filepath.Join(configDir, "configmaps", ref.Name),
})
```

#### Reloading on config changes

There can be 3 major types of changes to configuration,

1. HiveConfig changed and therefore the serialized file mounted using ConfigMap will also
    be updated in place by the kubelet.
2. One of mounted Secret/ConfigMap had some key removed or added.
3. One of mounted Secret/ConfigMap had an existing key's contents change.

To support the changing configuration due to content change i.e. (1) or (3), we use a fileobserver
that watches the ConfigDir and restarts the pod whenever the contents change for the existing files.

To support the addition or removal of certain data files i.e. (2), we create another fileobserver that
periodically creates a list of all the files in ConfigDir, and if the list changes it restarts the
pod.

#### Updating controllerutils

We must update the pkg/controllerutils to supports these actions,

1. Load HiveConfig from file in the `filepath.Join(ConfigDir, "hiveconfig", "config.yaml")`.
2. Load controller-runtime controller configuration for a given controller.
3. Load a Secret from disk, given secret name return the data (`map[string][]byte`) from
    `filepath.Join(ConfigDir, "secrets", secretName)` directory.
4. Load a ConfigMap from disk, given configmap name return the data (`map[string]string`) from
    `filepath.Join(ConfigDir, "configmaps", secretName)` directory.

### Risks and Mitigations

### Open Questions [optional]

### Upgrade Strategy

## Drawbacks

## Alternatives

### ComponentConfigs using controller-runtime

Instead of giving controllers serialized HiveConfig we could create a Component config for controllers
like recommended in [doc](https://github.com/kubernetes-sigs/controller-runtime/tree/master/examples/configfile/custom).
But creating a new internal type adds overhead of transforming HiveConfig to this type and therefore does not
enabled our goal (1).
