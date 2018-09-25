# OpenShift Hive
API driven OpenShift cluster provisioning and management

## Development Workflow

* Create the ClusterDeployment CRD:
  * `$ kubectl apply -f config/crds/hive_v1alpha1_clusterdeployment.yaml --validate=false`
* Run the Hive controllers from source:
  * `$ make run`
* Create a ClusterDeployment:
  * Assuming AWS credentials set in the standard environment variables, and our usual SSH key.
  * `$ oc process -f config/templates/cluster-deployment.yaml CLUSTER_NAME=$USER ADMIN_EMAIL="dgoodwin@redhat.com" ADMIN_PASSWORD="letmein" SSH_KEY="$(cat ~/libra.pem)" AWS_ACCESS_KEY_ID="$(echo $AWS_ACCESS_KEY_ID | base64)" AWS_SECRET_ACCESS_KEY="$(echo $AWS_SECRET_ACCESS_KEY | base64)" | oc apply -f -`
* Delete your ClusterDeployment:
  * `$ oc delete clusterdeployment $USER`

