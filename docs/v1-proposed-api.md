# Proposed Changes for Hive V1 API

 1. Require an InstallConfig ConfigMap, referenced in a new ClusterDeployment.Spec field, to pass directly to installer.
   * Remaining ClusterDeployment.Spec should be mostly, things that can be modified post-install.
   * Would allow quicker exposure to new Installer features without needing to mirror in Hive.
   * Must watch out for secrets (pull secret?) that should not be stored in ConfigMap. If it's just PullSecrets, this might be ok, but otherwise we may need to have Hive logic to inject these.
 1. Adjust HiveConfig API to map managed DNS domains to cloud provider they should be used for.
   * Would allow using Hive to provision to multiple clouds, using that cloud for DNS forwarding, from one Hive.
   * This is not the expected deployment model in Dedicated (one Hive per cloud)
 1. Remove deprecated Status.Installed in favor of newer Spec.Installed.
 1. Rename ClusterDeprovisionRequest to ClusterDeprovision, to better match ClusterProvision.
