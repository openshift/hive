#!/bin/bash

set -e

usage(){
	echo "Usage: $0 [KUBECONFIG] [STARTINDEX] [ENDINDEX]"
	exit 1
}


# Get the duplication count
START=${2:-1}
if [[ "${START}" -le 0 ]]
then
	echo "STARTINDEX must be a positive integer: ${START}"
	usage
	exit 1
fi

END=${3:-1}
if [[ "${END}" -le 0 ]]
then
	echo "ENDINDEX must be a positive integer: ${END}"
	usage
	exit 1
fi

if [[ "${END}" -le "${START}" ]]
then
	echo "ENDINDEX must be greater than STARTINDEX"
	usage
	exit 1
fi


for (( i=${START}; i<=${END}; i++ ))
do
	cluster_name="c${i}"
	ns="ns${i}"
	echo "Creating cluster: ${ns}/${cluster_name}"
	oc create namespace ${ns} || true
	bin/hiveutil create-cluster \
		--namespace=${ns} \
		--adopt \
		--adopt-admin-kubeconfig=${1} \
		--adopt-infra-id="fake-${cluster_name}" \
		--adopt-cluster-id="fake-${cluster_name}" \
		-l scaletest=true --skip-machine-pools \
		${cluster_name}
done

