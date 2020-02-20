#!/bin/bash
#
# This script will print to stdout the kubeconfig for a ClusterDeployment.
#

set -e

usage(){
	echo "Usage: $0 [CLUSTERDEPLOYMENT_NAMESPACE/]CLUSTERDEPLOYMENT_NAME"
	exit 1
}


# Get the name and namespace for the clusterdeployment to duplicate.
CD_NAME_ARG=${1?missing ClusterDeployment name}
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
	echo "Could not determine namespace and name of ClusterDeployment"
	usage
	;;
esac

# Argument to supply to oc commands to provide the namespace. This is empty if
# the user did not specify a namespace.
NAMESPACE_ARG="${CD_NAMESPACE:+--namespace }${CD_NAMESPACE}"

oc extract ${NAMESPACE_ARG} secret/$(oc get cd ${NAMESPACE_ARG} ${CD_NAME} -o jsonpath='{.spec.clusterMetadata.adminKubeconfigSecretRef.name}') --keys=kubeconfig --to=-
