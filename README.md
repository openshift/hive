# OpenShift Hive
API driven OpenShift cluster provisioning and management

# Create the ClusterDeployment Custom Resource Definition (CRD)

  `$ kubectl apply -f config/crds/hive_v1alpha1_clusterdeployment.yaml --validate=false`

# Create a Cluster Deployment

  `$ oc process -f config/templates/cluster-deployment.yaml CLUSTER_NAME=$USER ADMIN_EMAIL="dgoodwin@redhat.com" ADMIN_PASSWORD="letmein" SSH_KEY="$(cat ~/libra.pem)" | oc apply -f -`

