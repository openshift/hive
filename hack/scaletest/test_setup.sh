#!/bin/bash

set -e

usage(){
	echo "Usage: $0 [KUBECONFIG] [COUNT]"
	exit 1
}


# Get the duplication count
COUNT=${2:-1}
if [[ "${COUNT}" -le 0 ]]
then
	echo "COUNT must be a positive integer: ${COUNT}"
	usage
	exit 1
fi

echo "Adopting cluster kubeconfig ${COUNT} times"

for (( i=0; i<${COUNT}; i++ ))
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
		${cluster_name}
	oc label cd -n ${ns} ${cluster_name} scaletest=true --overwrite=true

	#for (( j=0; j<10; j++ ))
	#do
		#oc process -f hack/scaletest/syncset-template.yaml CLUSTER_NAME=${cluster_name} CLUSTER_NAMESPACE=${ns} RESOURCE_INDEX="${j}" | oc apply -f -
	#done

done

