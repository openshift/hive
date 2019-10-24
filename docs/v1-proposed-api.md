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


 1. Add Spec.InstallConfigSecret containing a full openshift-install InstallConfig  to pass directly to installer.
   * This will allow immediate use of new openshift-install features without needing to mirror in Hive's API.
   * Ideally most items in ClusterDeployment.Spec should be assumed mutable, however this will not universally be true.
   * There will be some overlap, information that must be provided in both the InstallConfig and the ClusterDeployment. We will attempt to error if you mismatch this information whenever possible.
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

CRD versioning functionality is not available in OpenShift 3.11 where prod/stage run and will continue to do so until private cluster support is ready and we can migrate. We must however move on V1 now.

Possible solutions:

  1. Fork the codebase. Branch for dedicated and backport critical fixes only. Once migrated to 4.x they can upgrade to newer Hive and gain access to the V1 API.
  2. Introduce a new parallel APIGroup.
    * Duplicate all controllers for the new types.
    * Coordinate move to V1 API with all dependent components. (OCM, cert operator, pagerduty operator, possibly others)
    * Maintain both sets of CRDs and controllers until the above coordination is complete.
    * Implement migration in hive-operator to port all legacy Cluster data to the new API.
    * Drop support for old CRs and controllers, remove migration code.
