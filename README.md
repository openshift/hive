# OpenShift Hive
API driven OpenShift cluster provisioning and management

# Create a Cluster Deployment

  `$ oc process -f config/templates/cluster-deployment.yaml CLUSTER_NAME=$USER ADMIN_EMAIL="dgoodwin@redhat.com" ADMIN_PASSWORD="letmein" SSH_KEY="$(cat ~/libra.pem)" | oc apply -f -`

