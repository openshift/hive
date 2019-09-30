#!/bin/sh
#
# This script can be used to extract the failure cluster logs tarball from the PVC created for an install pod.
# Note that there will only be failure logs if there was a failure installing and that the failure logs will
# only be retained for 7 days after a successful install.
#
# Usage: ./logextractor.sh [CLUSTERDEPLOYMENT_NAMESPACE/]CLUSTER_DEPLOYMENT_NAME DEST_DIR
#

usage(){
	echo $1
	echo "Usage: $0 [CLUSTERDEPLOYMENT_NAMESPACE/]CLUSTERDEPLOYMENT_NAME DEST_DIR"
	exit 1
}

# Get the name and namespace for the clusterdeployment.
CD_NAME_ARG=${1}
[ -z ${CD_NAME_ARG} ] && usage "missing ClusterDeployment name"
CD_NAME_ARR=(${CD_NAME_ARG//// })
case ${#CD_NAME_ARR[@]} in
1)
	CD_NAME=${CD_NAME_ARR[0]}
	;;
2)
	CD_NAMESPACE=${CD_NAME_ARR[0]}
	CD_NAME=${CD_NAME_ARR[1]}
	;;
*)
	usage "Could not determine namespace and name of ClusterDeployment"
	;;
esac

# Get the destination directory
DEST_DIR=${2}
[ -z ${DEST_DIR} ] && usage "missing destination directory"

# Argument to supply to oc commands to provide the namespace. This is empty if
# the user did not specify a namespace.
NAMESPACE_ARG="${CD_NAMESPACE:+--namespace }${CD_NAMESPACE}"

POD_NAME="hive-logextractor"
PVC_NAME=$(oc get pvc ${NAMESPACE_ARG} -l hive.openshift.io/cluster-deployment-name=${CD_NAME} -o jsonpath="{.items[0].metadata.name}" 2> /dev/null)

if [ -z "$PVC_NAME" ]
then
	echo "Could not find PVC for ClusterDeployment ${CD_NAME_ARG}."
	echo "Either the installation has not yet started for the ClusterDeployment, the installation succeeded on the first attempt, or the successful installation was long enough ago that the PVC has already been cleaned up."
	exit 1
fi

echo "Found PVC for cluster deployment: $PVC_NAME"

echo "apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME${CD_NAMESPACE:+
  namespace: }${CD_NAMESPACE}
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
    terminationMessagePolicy: File" | \
oc apply -f -

echo "waiting for $POD_NAME pod to be ready"

oc wait --for=condition=Ready --timeout=120s ${NAMESPACE_ARG} pod/$POD_NAME

echo "copying logs to ${DEST_DIR}"
oc cp ${NAMESPACE_ARG} $POD_NAME:/logs ${DEST_DIR}

echo "deleting logextractor pod"
oc delete pod ${NAMESPACE_ARG} $POD_NAME
