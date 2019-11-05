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


 1. Add Spec.Installation struct to contain install specific, immutable settings.
 1. Add Spec.Installation.InstallConfigSecret containing a full openshift-install InstallConfig  to pass directly to installer.
   * This will allow immediate use of new openshift-install features without needing to mirror in Hive's API.
   * Ideally most items in ClusterDeployment.Spec should be assumed mutable, however this will not universally be true.
   * There will be some overlap, information that must be provided in both the InstallConfig and the ClusterDeployment. We will attempt to error if you mismatch this information whenever possible.
 1. Add Spec.Installation.ManifestsConfigMap for injecting additional manifests into the install process.
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
 1. Introduce new modes of managing MachineSets in cluster.
   * Current "MachinePool" model of a simplified struct that is processed into a set of MachineSets spread across AZs will remain to simplify life for OCM and users who don't want fine grained control over their MachineSets.
   * Introduce a new Hive CRD *MachineSetGenerator* which will contain the previous MachinePool structure borrowed from the installer. One MachineSet generator should tie to one machine pool.
   * Users such as OCM wishing to use simplified MachineSet management will need to create this new CR instead of embedding information in the ClusterDeployment.
   * After install, Hive will sync back the MachineSets generated by the installer and store them in a Hive SyncSet. One MachineSet SyncSet will be created per MachinePool. This sync back will only happen once, after which the SyncSet becomes authoritative.
   * If a MachineSetGenerator exists, it will reconcile against the contents of this SyncSet.
   * If no MachineSetGenerator exists, the MachineSet SyncSet can be manually edited by a Hive API caller to fully control their MachineSets.
 1. Autoscaling can be configured for a particular MachineSet via a [SyncSet](https://docs.openshift.com/container-platform/4.2/machine_management/applying-autoscaling.html#machine-autoscaler-cr_applying-autoscaling)
 1. Machine Config Pools can be configured for a particular MachineSet of nodes with a label via a [SyncSet](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigController.md#machineconfigpool)
   * TODO: Does this need an initial sync back to hive post-install?


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
  compute: # controlPlane is gone, install config only, but may reappear here if these fall under machine set management in future.
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
  installation:
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

To avoid having to fork and abandon stage/prod until they can move to 4.3+ we propose the following solution using an aggregated API server. The migration will be orchestrated by the hive-operator.

  1. Begin V1 work in v1 branch. Will merge back to master once we have the migration code ready. (may be a period of a few weeks)
  1. hive-operator migration
    1. Scale down v1alpha1 controllers pod.
    1. Deploy an admission validating webhook that blocks all mutating actions on v1alpha1 CRDs.
      * This will block all integrating applications from making any changes while data is being migrated.
      * WARNING: we're not sure this works, the removal of finalizers is an update, we'd block ourselves.
    1. Delete the v1alpha1 controller deployments.
    1. Deploy the new v1 Hive CRDs.
      * These will have the same names as in v1alpha1 but reside in a new API group.
    1. Migrate v1alpha1 CRs to v1.
      * We will create an InstallConfig secret for all legacy ClusterDeployments during this process. This ensures no data is lost and API responses remain consistent.
    1. Remove finalizers on v1alpha1 objects to help ensure the controllers do not attempt to deprovision or perform any actions when we delete.
    1. Delete all v1alpha1 custom resources.
      * Finalizers should be deleted and controllers scaled down, so no action should be taken.
    1. Delete all v1alpha1 CRDs, at this point the API is temporarily down for anyone using v1alpha1.
    1. Delete the blocking webhook.
    1. Deploy the aggregated API server to handle v1alpha1 API requests.
      * Aggregated API will serve v1alpha1 exactly as we do today, no change is required for callers such as OCM and SRE operators.
      * Aggregated API will forward all requests to the Hive v1 CRs.
      * Aggregated API must be able to handle an incoming create requests which do not have InstallConfig referenced. We will need to generate an InstallConfig, save to a Secret, and then forward the translated V1 object to the normal API server.
    1. Deploy the new V1 hive controllers deployment.

Benefits:

  * No forking of codebase required.
  * All Hive code outside the aggregated API server is using the new types.
  * Once everyone is off v1alpha1, we can simply delete the aggregated API server to remove v1alpha1.

Concerns:

  * Must be very careful nothing can possibly be listening when we do the migration, particularly when deleting the old CRs, which would trigger a cluster deprovision. However if finalizers are removed and controllers are scaled down, this should not be possible.
  * We will want to ensure callers are migrating off v1alpha1 in a timely manner to limit maintenance overhead of the aggregated api.

