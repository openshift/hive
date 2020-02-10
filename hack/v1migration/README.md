# Migration Testing Steps

  1. Deploy master (v1alpha1) to a cluster.
  1. Create a cluster deployment and wait for completion.
  1. Run `./hack/v1migration/scaledown.sh` to scale down Hive deployments.
  1. Run `./hack/v1migration/delete.sh [workdir]` to backup all data to disk and remove Hive resources (CRs, CRDs, and admission webhooks).
  1. Deploy Hive v1.
  1. If running on a cluster that does not support cert injection, run `./hack/hiveapi-dev-certs.sh` to create the certs for the Hive aggregated API server.
  1. Run `./hack/v1migration/deploy_apiserver.sh` to enabled the Hive aggregated API server.
  1. Run `./hack/v1migration/restore_hiveconfig.sh [workdir]` to restore HiveConfig from v1alpha1.
  1. Wait for any changes from HiveConfig for hiveadmission to propagate.
  1. Run `./hack/v1migration/scaledown.sh` to scale down Hive deployments.
  1. Run `./hack/v1migration/restore.sh [workdir]` to restore all data from disk.
  1. Run `./hack/v1migration/scaleup.sh` to scale up Hive deployments.
