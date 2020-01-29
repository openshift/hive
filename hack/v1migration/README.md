# Migration Testing Steps

  1. Deploy master (v1alpha1) to a cluster.
  1. Create a cluster deployment and wait for completion.
  1. Run `./hack/v1migration/scaledown.sh` to scale down Hive deployments.
  1. Run `./hack/v1migration/delete.sh [workdir]` to backup all data to disk and remove Hive resources (CRs, CRDs, and admission webhooks).
  1. Deploy Hive v1
  1. ./hack/make-apiserver-certs.sh
  1. ./hack/deploy-apiserver.sh
  1. Run `./hack/v1migration/scaledown.sh` to scale down Hive deployments.
  1. Run `./hack/v1migration/restore.sh [workdir]` to restore all data from disk.
  1. Run `./hack/v1migration/scaleup.sh` to scale up Hive deployments.

# Known issues

  * Aggregated API server is very slow in serving ClusterDeployment.
  * Status is not getting set on migrated objects. The aggregated API server does not accept status updates. This will 
  show up as errors when running the restore.sh script.
