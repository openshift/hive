#!/bin/bash


namespace="${1:-hive}"

IMG="localhost:5000/hive:latest"

echo "Creating namespace 'hive' if it doesn't exist..."
kubectl create namespace ${namespace} || true

echo "Creating deploy directory and copying kustomization.yaml..."
mkdir -p overlays/deploy
cp overlays/template/kustomization.yaml overlays/deploy


cd overlays/deploy

echo "Setting image and namespace in kustomization.yaml..."
kustomize-4.1.3 edit set image registry.ci.openshift.org/openshift/hive-v4.0:hive=${IMG}
kustomize-4.1.3 edit set namespace ${namespace}

cd ../../

echo "Building and applying kustomize configuration..."
kustomize-4.1.3 build overlays/deploy | sed 's/        - info/        - debug/' | oc apply -f -

echo "Cleaning up deploy directory..."
rm -rf overlays/deploy

echo "Applying CRDs..."
kubectl apply -f config/crds

echo "Creating default HiveConfig..."
cd config/templates/
oc process --local=true -p HIVE_NS=${namespace} -p LOG_LEVEL=debug -f hiveconfig.yaml | oc apply -f -



echo "Operator deployment completed successfully."

