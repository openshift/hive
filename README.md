# OpenShift Hive
API driven OpenShift cluster provisioning and management

## Deploying Locally

* Ensure that you have access to an OpenShift cluster and have administrator access

* Build and provision CRDs, roles, and controller:
   * `$ make provision`

## Development Workflow

* Create the ClusterDeployment CRD:
  * `$ kubectl apply -f config/crds/hive_v1alpha1_clusterdeployment.yaml --validate=false`
* Run the Hive controllers from source:
  * `$ make run`
* Create a ClusterDeployment:
  * Place the OpenShift images pull secret in a known location like `$HOME/config.json`
  * Assuming AWS credentials set in the standard environment variables, and our usual SSH key.
  ```bash
  export CLUSTER_NAME="${USER}"
  export ADMIN_EMAIL="${USER}@redhat.com"
  export ADMIN_PASSWORD="letmein"
  export SSH_PUB_KEY="$(ssh-keygen -y -f ~/.ssh/libra.pem)"
  export PULL_SECRET="$(cat ${HOME}/config.json)"
  oc process -f config/templates/cluster-deployment.yaml \
     CLUSTER_NAME="${CLUSTER_NAME}" \
     ADMIN_EMAIL="${ADMIN_EMAIL}" \
     ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
     SSH_KEY="${SSH_PUB_KEY}" \
     PULL_SECRET="${PULL_SECRET}" \
     AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
     AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
     | oc apply -f -
  ```
* Delete your ClusterDeployment:
  ```bash
  $ oc delete clusterdeployment $USER
  ```

## Tips

### Using the Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
kubectl get secret ${USER}-admin-kubeconfig -o json | jq ".data.kubeconfig" -r | base64 -d > ${USER}.kubeconfig
export KUBECONFIG=${USER}.kubeconfig
kubectl get nodes
```
