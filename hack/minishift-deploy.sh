#!/bin/sh
set -e
HIVE_DOCKER_CMD="docker --tls --tlscacert=$HOME/.minishift/certs/ca.pem --tlscert=$HOME/.minishift/certs/cert.pem --tlskey=$HOME/.minishift/certs/key.pem"
eval $(minishift docker-env)
DOCKER_CMD=$HIVE_DOCKER_CMD make docker-build
IMG=hive-controller:latest
minishift_registry="$(minishift openshift registry)"
current_project="$(oc project -q)"
#$HIVE_DOCKER_CMD login -u admin -p "$(oc whoami -t)" $minishift_registry
#$HIVE_DOCKER_CMD tag ${IMG} $minishift_registry/$current_project/${IMG}
#$HIVE_DOCKER_CMD push $minishift_registry/$current_project/${IMG}
kustomize build config/default | kubectl apply -f -
kubectl delete pod -l deploymentconfig=hive-controller-manager
