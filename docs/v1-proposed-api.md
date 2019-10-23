# Proposed Changes for Hive V1 API

## ClusterDeployment

 1. Require an InstallConfig ConfigMap, referenced in a new ClusterDeployment.Spec field, to pass directly to installer.
   * GOAL: Allow quicker exposure to new Installer features without needing to mirror in Hive's API.
   * GOAL: Remaining ClusterDeployment.Spec should be things that can be modified post-install.
   * Should Hive parse the InstallConfig to obtain information it needs, eliminating the need for the caller to duplicate information in ClusterDeployment and InstallConfig?
     * Or should Hive be explicitly given all the information it needs to adopt and maintain a cluster in it's ClusterDeployment, requiring the caller to duplicate a fair portion of InstallConfig in each.
     * Should we use an InstallConfig if given one, otherwise generate?
   * Add Spec.InstallConfigMap reference.
   * Likely cannot embed pull secret and ssh key into the InstallConfig in a ConfigMap.
     * Should we require the InstallConfig in a Secret instead?
     * Should we template in the secret values on the fly?
   * It does not appear that we can remove much of our API. Only Spec.Networking can go. We still need the platform for region, the pull secret + ssh key, the machine sets for on-going management and syncing. There is actually not that much config exposed otherwise.
     * Doing this would still expose installer features faster, however the caller would have to be aware of two APIs instead of just one, including multiple versions of InstallConfig, and duplicate a bit of information in both.
 1. Remove deprecated Status.Installed in favor of newer Spec.Installed.
 1. Drop ClusterDeployment.Spec.Images, this is unused and unnecessary.
 1. Rename ClusterDeployment.Spec.SSHKey to ClusterDeployment.Spec.SSHKeySecret to better match all other secret references.
 1. Allow adoption of a cluster without requiring creation of a ClusterProvision:
   * If we didn't provision it we should not need a ClusterProvision.
   * Move refs to secrets with admin kubeconfig/password into Spec instead of Status.
   * Move Status.ClusterID/Infra ID to Spec instead of Status.
 1. Add support for bare metal provisioning.
   * This is modelled as another platform in InstallConfig. No reason we shouldn't do the same.

## ClusterProvision

 1. Drop PodSpec, generate on the fly? (note: this is not within the Spec, external, possibly already generated on fly)

## ClusterDeprovisionRequest

 1. Rename ClusterDeprovision, to better match ClusterProvision.

## SyncSet + SelectorSyncSet

 1. Drop support for ApplyOnce patch mode.

## HiveConfig

 1. Adjust HiveConfig API to map managed DNS domains to cloud provider they should be used for.
   * Would allow using Hive to provision to multiple clouds, using that cloud for DNS forwarding, from one Hive.
   * This is not the expected deployment model in Dedicated (one Hive per cloud)
   * Convert managedDomains to a list of structs rather than just a string. Struct should include a list of the platforms we will manage DNS for on this sub-domain.
 1. Rename ExternalDNS to ManageDNS (section where the credentials are stored, ExternalDNS being phased out)

# Current V1Alpha1 ClusterDeployment Example

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
  infraID: ci-cluster-v4-1-64bns
  installed: true
  installedTimestamp: "2019-10-23T08:38:03Z"
  installerImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:c9c1e6d1897b74d366d331f5cdc3fa16c5a96abe28ff8238833c09042156e983
  provision:
    name: ci-cluster-v4-1-0-ndv7l
  webConsoleURL: https://console-openshift-console.apps.ci-cluster-v4-1.r4f1.s1.devshift.org
```



