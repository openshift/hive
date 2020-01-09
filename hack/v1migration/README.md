# Migration Testing Steps

  1. Deploy master (v1alpha1) to a cluster.
  1. Create a cluster deployment and wait for completion.
  1. Run ./hack/v1migration/phase1.sh [workdir] to scale down pods and backup all data to disk, and remove finalizers.
  1. bin/hiveutil v1migration remove-secret-owner-refs
  1. Run ./hack/v1migration/phase2.sh [workdir] to delete all Hive CRs and CRDs.
  1. Deploy Hive v1
  1. ./hack/make-apiserver-certs.sh
  1. ./hack/deploy-apiserver.sh
  1. Scale everything down to 0 again, but keep hiveadmission and hiveapi up.
  1. kubectl apply -f [workdir] # run twice due to problems recognizing types initially?

## TODO

  1. Restored ClusterDeployment has no infraID.
  1. Restored ClusterProvision has no owner reference. (probably many other types as well)
  1. Admin kubeconfig/password secrets have no owner reference after we cleared it.


