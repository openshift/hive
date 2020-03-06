# Migration Testing Steps

  1. Deploy master (v1alpha1) to a cluster.
  1. Create a cluster deployment and wait for completion.
  1. Run `oc patch hiveconfig hive --type='merge' -p $'spec:\n maintenanceMode: true'`
  1. Run `./hack/v1migration/scaledown.sh hive-operator` to scale down Hive operator.
  1. Run `./hack/v1migration/delete_hive_jobs.sh` to delete any outstanding Hive jobs (imageset, install, uninstall).
  1. Run `./hack/v1migration/save.sh [workdir]` to backup all data to disk.
  1. Run `./hack/v1migration/remove_hiveadmission.sh` to remove the Hive validation webhooks.
  1. Run `./hack/v1migration/delete.sh [workdir]` to remove Hive resources (CRs and CRDs).
  1. Deploy Hive v1.
  1. If running on a cluster that does not support cert injection, run `./hack/hiveapi-dev-certs.sh` to create the certs for the Hive aggregated API server.
  1. Run `./hack/v1migration/deploy_apiserver.sh` to enabled the Hive aggregated API server.
  1. Run `./hack/v1migration/restore_hiveconfig.sh [workdir]` to restore HiveConfig from v1alpha1.
  1. Wait for any changes from HiveConfig for hiveadmission to propagate.
  1. Run `oc patch hiveconfig hive --type='merge' -p $'spec:\n maintenanceMode: true'`
  1. Run `./hack/v1migration/scaledown.sh hive-operator` to scale down Hive operator.
  1. Run `./hack/v1migration/restore.sh [workdir]` to restore all data from disk.
  1. Run `./hack/v1migration/scaleup.sh hive-operator` to scale up Hive operator.
  1. Run `oc patch hiveconfig hive --type='merge' -p $'spec:\n maintenanceMode: false'`


# Resolving Problems during Restore

If there was a problem during restore, then we should delete the resources in that namesapce
that were restored, fix the data being restored, and run the restore again for just that namespace.

  1. `./hack/v1migration/hive_resources_in_namespace.sh [workdir] [namespace] > [namespace_workdir]/resources.json`
  1. `./hack/v1migration/hive_owner_refs_in_namespace.sh [workdir] [namespace] > [namespace_workdir]/owner-ref.json`
  1. `./hack/v1migration/delete_in_namespace.sh [namespace]`
  1. Make whatever changes are needed to resources saved in `[namespace_workdir]`.
  1. `./hack/v1migration/restore.sh [namespace_workdir]`