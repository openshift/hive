# OpenShift Hive
API driven OpenShift cluster provisioning and management

# Create the ClusterDeployment Custom Resource Definition (CRD)

  `$ kubectl apply -f config/crds/hive_v1alpha1_clusterdeployment.yaml --validate=false`

# Create a Cluster Deployment

Assuming AWS credentials set in the standard environment variables, and our usual SSH key:

  `$ oc process -f config/templates/cluster-deployment.yaml CLUSTER_NAME=$USER ADMIN_EMAIL="dgoodwin@redhat.com" ADMIN_PASSWORD="letmein" SSH_KEY="$(cat ~/libra.pem)" AWS_ACCESS_KEY_ID="$(echo $AWS_ACCESS_KEY_ID | base64)" AWS_SECRET_ACCESS_KEY="$(echo $AWS_SECRET_ACCESS_KEY | base64)" | oc apply -f -`

