#!/bin/sh
#
# This script can be used to extract the failure cluster logs tarball from the PVC created for an install pod.
#
# Usage: ./logextractor.sh [CLUSTER_DEPLOYMENT_NAME] [DEST_DIR]
#

POD_NAME="hive-logextractor"
PVC_NAME=$(oc get pvc -l hive.openshift.io/cluster-deployment-name=$1 -o jsonpath="{.items[0].metadata.name}" 2> /dev/null)

if [ -z "$PVC_NAME" ]; then echo "Could not find PVC for ClusterDeployment $1"; exit 1; fi

echo "Found PVC for cluster deployment: $PVC_NAME"

echo "apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
spec:
  volumes:
  - name: logsvol
    persistentVolumeClaim:
      claimName: $PVC_NAME
  containers:
  - image: centos:7
    command: ["/bin/sh"]
    args: ["-c", "while true\; do sleep 100000000\; done"]
    imagePullPolicy: Always
    name: logextractor
    resources: {}
    volumeMounts:
    - mountPath: "/logs"
      name: logsvol
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File" | oc apply -f -


echo "waiting for $POD_NAME pod to be ready"

oc wait --for=condition=Ready --timeout=120s pod/$POD_NAME

echo "copying logs to $2"
oc cp $POD_NAME:/logs $2

echo "deleting logextractor pod"
oc delete pod $POD_NAME
