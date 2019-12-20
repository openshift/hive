# Proposed Changes for Hive V1 API

Minimal changes, no new CRDs or relationships between.

## ClusterDeployment

### Current V1Alpha1 ClusterDeployment Example

```yaml
apiVersion: hive.openshift.io/v1alpha1
kind: ClusterDeployment
metadata:
  name: ci-cluster-v4-1
  namespace: uhc-staging-18urm2sbab85d9crlk27is39n706ph3g
spec:
  baseDomain: r4f1.s1.devshift.org
  certificateBundles:
  - generate: true
    name: primary-cert-bundle
    secretRef:
      name: primary-cert-bundle-secret
  clusterName: ci-cluster-v4-1
  compute:
  - name: worker
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 300
          type: gp2
        type: m5.xlarge
        zones:
        - us-east-1a
    replicas: 4
  controlPlane:
    name: master
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 32
          type: gp2
        type: m5.xlarge
        zones:
        - us-east-1a
    replicas: 3
  controlPlaneConfig:
    servingCertificates:
      default: primary-cert-bundle
  imageSet:
    name: openshift-v4.1.18
  images: {}
  ingress:
  - domain: apps.ci-cluster-v4-1.r4f1.s1.devshift.org
    name: default
    servingCertificate: primary-cert-bundle
  installed: true
  manageDNS: true
  networking:
    clusterNetworks:
    - cidr: 10.128.0.0/14
      hostSubnetLength: 23
    machineCIDR: 10.0.0.0/16
    serviceCIDR: 172.30.0.0/16
    type: OpenShiftSDN
  platform:
    aws:
      region: us-east-1
  platformSecrets:
    aws:
      credentials:
        name: aws
  pullSecret:
    name: pull
  sshKey:
    name: ssh
status:
  adminKubeconfigSecret:
    name: ci-cluster-v4-1-0-ndv7l-admin-kubeconfig
  adminPasswordSecret:
    name: ci-cluster-v4-1-0-ndv7l-admin-password
  apiURL: https://api.ci-cluster-v4-1.r4f1.s1.devshift.org:6443
  certificateBundles:
  - generated: true
    name: primary-cert-bundle
  cliImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:528f2ead3d1605bdf818579976d97df5dd86df0a2a5d80df9aa8209c82333a86
  clusterID: 1e4f00a3-a30e-46d7-9567-4bb61caf6d73
  clusterVersionStatus:
    [SNIP]
  conditions:
  - lastProbeTime: "2019-10-23T08:40:24Z"
    lastTransitionTime: "2019-10-23T08:40:24Z"
    message: Control plane certificates are present
    reason: ControlPlaneCertificatesFound
  installed: true
  installedTimestamp: "2019-10-23T08:38:03Z"
  installerImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:c9c1e6d1897b74d366d331f5cdc3fa16c5a96abe28ff8238833c09042156e983
  provision:
    name: ci-cluster-v4-1-0-ndv7l
  webConsoleURL: https://console-openshift-console.apps.ci-cluster-v4-1.r4f1.s1.devshift.org
```

### Proposed Changes


  1. Add Spec.Provisionin struct to contain install specific, immutable settings.
  1. Add Spec.Provisionin.InstallConfigSecret containing a full openshift-install InstallConfig  to pass directly to installer.
    * This will allow immediate use of new openshift-install features without needing to mirror in Hive's API.
    * Ideally most items in ClusterDeployment.Spec should be assumed mutable, however this will not universally be true.
    * There will be some overlap, information that must be provided in both the InstallConfig and the ClusterDeployment. We will attempt to error if you mismatch this information whenever possible.
  1. Add Spec.Provisionin.ManifestsConfigMap for injecting additional manifests into the install process.
    * Can be used for something like switching to Calico for networking.
    * ConfigMap with a file in each key. Will mount the whole configmap and iterate each file, adding to the installer's manifests dir.
  1. Remove:
    * Spec.ClusterName - In install config, not relevant thereafter.
      * May be used in some naming, but hopefully can transition to CD.Name instead.
    * Spec.ControlPlane - Not relevant after install today as masters are not a MachineSet. Can be re-added in future if necessary.
    * Spec.Networking
  1. Remaining immutable ClusterDeployment.Spec fields:
    * Spec.ImageSet and Spec.Images. (technically could be mutable but has no effect once we've installed)
    * Spec.Platform.AWS.Region (deemed important enough to be present in spec)
    * ManageDNS bool: Enabled automated DNS forwarding. May be removed in V2.
    * BaseDomain: Used to drive the automatic creation of the DNSZone if manageDNS is true. May be removed in V2.
  1. Introduce Spec.ClusterMetadata struct with fields for InfraID, ClusterUUID, and admin password/kubeconfig secret references.
    * All of these fields will be removed from Status where they cannot be restored.
    * InfraID and ClusterUUID will be programatically mutable, but once set and the cluster is marked installed, validation will make them immutable.
    * ClusterMetadata may be renamed if a better idea comes up.
  1. Rename ClusterDeployment.Spec.SSHKey to ClusterDeployment.Spec.SSHKeySecret to better match all other secret references.
    * Should not be removed as private keys are not contained in InstallConfig and are used by Hive.
    * Rename to SSHPrivateKeySecret and should only contain private key going forward.
  1. Rename CertificateBundle.SecretRef to CertificateSecret.
  1. Move PlatformSecrets into Platform, rename Credentials to CredentialsSecret.
  1. Move MachinePool definitions out of ClusterDeployment and to a separate CRD.
    * Master pool will remain only in InstallConfig for now until we reach a point where masters are also an in-cluster MachineSet.
    * Change replicas to an optional field. This is to allow future support for specifying autoscaling config for the pool. (see below)
    * Future feature: Add support for [autoscaling](https://docs.openshift.com/container-platform/4.2/machine_management/applying-autoscaling.html#machine-autoscaler-cr_applying-autoscaling) here.
      * MachinePool.Spec.Autoscaling.MinReplicas and MaxReplicas.
      * Can exactly one of MachinePool.Spec.Replicas and MachinePool.Spec.Autoscaling.
      * Requested min/max would be broken down across all specified availability zones as we do for replicas today. If a user wants more control and less magic over this, they can create multiple MachinePools with just one AZ, each with it's own autoscaling config.
    * Future feature: Add support for configuring [MachineConfigPools](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigController.md#machineconfigpool) for this MachinePool.
  1. Add support for bare metal provisioning.
    * At this point, we do not see any major implications for BareMetal. This is modelled as a separate platform in install config (see [this example](https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md#install-config))
    * This would be specified in the InstallConfig.
    * Hive would likely want a platform indicator explicitly for bare metal if we need a special invocation of the installer.
    * However for the most part, we do not see any major complications adding bare metal support to the Hive API.
  1. Add support for hosted control plane provisioning. (control plane running in Hive management cluster, in the same namespace as the ClusterDeployment, but cluster workers in a remote cloud provider account)
    * A lot needs to be settled here before we can know exactly how this would look. However looking over the [current inputs for the POC code](https://github.com/sjenning/hosted-cluster-poc/blob/master/config.sh.aws_example), we will likely combine a Spec.Platform as we have today, with another Spec.HostedControlPlane section containing a variety of settings.
    * May deploy control plane to the same cluster as Hive, or give Hive a kubeconfig for a remote cluster where the control plane should run.
    * Safer to assume we are not the cluster where control plane will run.
    * Spec.Platform.AWS specified, as well as Spec.HostedControlPlane.Kubeconfig etc.


### Proposed V1 ClusterDeployment

```yaml
apiVersion: hive.openshift.io/v1alpha1
kind: ClusterDeployment
metadata:
  name: ci-cluster-v4-1
  namespace: uhc-staging-18urm2sbab85d9crlk27is39n706ph3g
spec:
  baseDomain: r4f1.s1.devshift.org
  certificateBundles:
  - generate: true
    name: primary-cert-bundle
    certificateSecret:
      name: primary-cert-bundle-secret
  controlPlaneConfig:
    servingCertificates:
      default: primary-cert-bundle
  imageSet: # immutable
    name: openshift-v4.1.18
  images: {} # immutable
  ingress:
  - domain: apps.ci-cluster-v4-1.r4f1.s1.devshift.org
    name: default
    servingCertificate: primary-cert-bundle
  clusterMetadata:
    # Mutable until installed=true, after which we will not allow updates.
    infraID: ci-cluster-v4-1-64bns
    clusterID: 1e4f00a3-a30e-46d7-9567-4bb61caf6d73
    adminKubeconfigSecret:
      name: ci-cluster-v4-1-0-ndv7l-admin-kubeconfig
    adminPasswordSecret:
      name: ci-cluster-v4-1-0-ndv7l-admin-password
  installed: true
  provisioning:
    installConfigSecret:
      name: cluster-install-config
    manifestsConfigMap:
      name: additional-install-manifests
  manageDNS: true # immutable
  platform:
    aws:
      region: us-east-1
      credentials:
        name: aws
  pullSecret:
    name: pull
status:
  apiURL: https://api.ci-cluster-v4-1.r4f1.s1.devshift.org:6443
  certificateBundles:
  - generated: true
    name: primary-cert-bundle
  cliImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:528f2ead3d1605bdf818579976d97df5dd86df0a2a5d80df9aa8209c82333a86
  clusterVersionStatus:
    [SNIP]
  conditions:
  - lastProbeTime: "2019-10-23T08:40:24Z"
    lastTransitionTime: "2019-10-23T08:40:24Z"
    message: Control plane certificates are present
    reason: ControlPlaneCertificatesFound
  installedTimestamp: "2019-10-23T08:38:03Z"
  installerImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:c9c1e6d1897b74d366d331f5cdc3fa16c5a96abe28ff8238833c09042156e983
  provision:
    name: ci-cluster-v4-1-0-ndv7l
  webConsoleURL: https://console-openshift-console.apps.ci-cluster-v4-1.r4f1.s1.devshift.org

apiVersion: hive.openshift.io/v1alpha1
kind: MachinePool
metadata:
  name: ci-cluster-v4-1-compute
  namespace: uhc-staging-18urm2sbab85d9crlk27is39n706ph3g
spec:
  name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 300
        type: gp2
      type: m5.xlarge
      zones:
      - us-east-1a
      - us-east-2a
  replicas: 4
  # future addition, would not be used in conjunction with replicas:
  autoscaling:
    minReplicas: 5
    maxReplicas: 10
```

## ClusterDeprovisionRequest

  1. Rename to ClusterDeprovision, to better match ClusterProvision.

## SyncSet + SelectorSyncSet

  1. Drop support for ApplyOnce patch mode.

## HiveConfig

  1. Adjust HiveConfig API to map managed DNS domains to cloud provider they should be used for.
    * Would allow using Hive to provision to multiple clouds, using that cloud for DNS forwarding, from one Hive.
    * This is not the expected deployment model in Dedicated (one Hive per cloud)
    * Convert managedDomains to a list of structs rather than just a string. Struct should include a list of the platforms we will manage DNS for on this sub-domain.
    * Could have multiple domains on AWS using different certs.
  1. Rename ExternalDNS to ManageDNS (section where the credentials are stored, ExternalDNS being phased out)


# Migrating to V1

CRD versioning functionality is not available in OpenShift 3.11 where prod/stage run and will continue to do so until private cluster support is ready and we can migrate. We must however move to V1 as soon as possible.

To avoid having to fork and abandon stage/prod until they can move to 4.3+ we propose the following solution using an aggregated API server.

Migration will be launched manually by SRE (with Hive team present), logged into the cluster as cluster admin. Likely a go program accompanied by shell scripts. After each phase we can examine state of cluster to ensure everything is progressing ok.

## Migration Steps

  1. Phase 1
    1. Scale down hive-operator, hive-controllers, and wait until no pods are running.
    1. Fetch yaml for all Hive v1alpha1 CRs and store on disk.
    1. Copy all yaml and manipulate for v1 CRs into a separate parallel directory structure we can diff.
      * Will require generating an InstallConfig secret, this is best done in go.
      * Will require breaking out MachinePools into separate  CRs. Not a good fit for jq, may be Python or Go.
  1. Phase 2
    1. Deploy all v1 Hive CRDs.
      * Ensure OLM will not choke when we deploy a new version of the CSV that carries v1 CRDs which are already deployed.
    1. oc apply our copied and modified yaml from disk.
    1. Examine state of cluster and ensure everything looks healthy.
  1. Phase 3 (beginning permanent changes)
    1. Update all owner references. (secrets, configmaps, etc)
    1. Remove all v1alpha1 finalizers. (ensure hive-controller is not running)
  1. Phase 4
    1. Delete all v1alpha1 CRs.
    1. Delete all v1alpha1 CRDs.
  1. Phase 5
    1. Merge v1 into master, triggering a deploy via OLM with v1 CRs and v1alpha1 aggregated API server.


Concerns:

  * Must be very careful nothing can possibly be listening when we do the migration, particularly when deleting the old CRs, which would trigger a cluster deprovision. However if finalizers are removed and controllers are scaled down, this should not be possible.
  * We will want to ensure callers are migrating off v1alpha1 in a timely manner to limit maintenance overhead of the aggregated api. 6 weeks planned window.

