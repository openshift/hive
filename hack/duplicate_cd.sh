#!/bin/bash
#
# This script will duplicate an installed clusterdeployment for testing purposes.
# The duplicated cluster will have a generated name that starts with the name
# of the original clusterdeployment followed by "-dup"
#

set -e

usage(){
	echo "Usage: $0 [CLUSTERDEPLOYMENT_NAMESPACE/]CLUSTERDEPLOYMENT_NAME [COUNT]"
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

# Get the duplication count
COUNT=${2:-1}
if [[ "${COUNT}" -le 0 ]]
then
	echo "COUNT must be a positive integer: ${COUNT}"
	usage
	exit 1
fi

echo "Duplicating ${CD_NAMESPACE}${CD_NAMESPACE:+/}${CD_NAME} ${COUNT} times"

# Argument to supply to oc commands to provide the namespace. This is empty if
# the user did not specify a namespace.
NAMESPACE_ARG="${CD_NAMESPACE:+--namespace }${CD_NAMESPACE}"

# Get the clusterdeployment to duplicate.
CD=$(oc get cd ${NAMESPACE_ARG} ${CD_NAME} -ojson)

# Ensure that the cluster has been installed.
if [ "$(echo ${CD} | jq .spec.installed)" != "true" ]
then
	echo "Cluster is not installed"
	exit 1
fi

# Get the clusterprovision to duplicate.
PROV=$(oc get clusterprovision ${NAMESPACE_ARG} $(echo ${CD} | jq -r .status.provision.name) -ojson)

# Create the json for the new clusterdeployment.
NEW_CD=$(echo ${CD} | jq \
	'del(.status) |
		.metadata|={
			"namespace":.namespace,
			"generateName":.name,
			"labels":{"hive.openshift.io/duplicated-from":.name}} |
		.metadata.generateName|=.+"-dup-" |
		.spec.installed=true |
		.spec.manageDNS=false |
		.spec.preserveOnDelete=true |
		.spec.certificateBundles[]?.generate=false')

# Create the json for the new clusterprovision, without the name or UID of the clusterdeployment.
NEW_PROV_BASE=$(echo ${PROV} | jq \
	'del(.status) |
		.metadata|={
			"namespace":.namespace,
			"ownerReferences":(.ownerReferences | select(.[].controller=true)),
			"labels":{"hive.openshift.io/duplicated-from":.name}} |
		.spec.stage="complete" |
		del(.spec.podSpec) |
		del(.spec.prevClusterID) |
		del(.spec.prevInfraID) |
		del(.spec.attempt)')
		
for (( i=0; i<${COUNT}; i++ ))
do
	# Create the new clusterdeployment, and save the name of the new clusterdeployment.
	CD_CREATE_OUTPUT=$(echo $NEW_CD | oc create -f -)
	echo ${CD_CREATE_OUTPUT}
	CD_CREATE_OUTPUT_ARR=(${CD_CREATE_OUTPUT//// })
	NEW_CD_NAME=${CD_CREATE_OUTPUT_ARR[1]}
	
	# Get the UID of the new clusterdeployment.
	NEW_CD_UID=$(oc get cd ${NAMESPACE_ARG} ${NEW_CD_NAME} -ojsonpath={.metadata.uid})
	
	# Fill out the clusterdeployment name and UID for the new clusterprovision.
	NEW_PROV=$(echo ${NEW_PROV_BASE} | jq \
		--arg name "${NEW_CD_NAME}" \
		--arg cd_uid "${NEW_CD_UID}" \
		'.metadata.name=$name |
			.metadata.ownerReferences[0].name=$name |
			.metadata.ownerReferences[0].uid=$cd_uid |
			.metadata.labels|=.+{"hive.openshift.io/cluster-deployment-name": $name}')
	
	# Create the new clusterprovision.
	echo $NEW_PROV | oc create -f -
done

