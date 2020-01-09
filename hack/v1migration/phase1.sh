#!/bin/sh

set -e

if [ $# != 1 ]; then
	echo "USAGE: $0 [WORKDIR]"
	exit 1
fi

WORKDIR=$1
echo "Using workdir: $WORKDIR"

oc project hive

oc scale -n hive deployment.v1.apps/hive-operator --replicas=0
oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0
oc scale -n hive deployment.v1.apps/hiveadmission --replicas=0

# TODO: wait until no pods are running

mkdir -p $WORKDIR/

HIVE_TYPES=( checkpoints clusterdeployments clusterdeprovisionrequests clusterimagesets clusterprovisions clusterstates dnsendpoints dnszones hiveconfig selectorsyncidentityprovider selectorsyncset syncidentityprovider syncsetinstance syncset )
for i in "${HIVE_TYPES[@]}"
do
	:
	echo "Storing all ${i} in ${WORKDIR}/${i}.yaml"
	oc get ${i}.hive.openshift.io -A -o json | jq '.items | .[] |
		del(.status) |
		del(.metadata.annotations."kubectl.kubernetes.io/last-applied-configuration") |
		del(.metadata.creationTimestamp) |
		del(.metadata.generation) |
		del(.metadata.resourceVersion) |
		del(.metadata.ownerReferences) |
		del(.metadata.selfLink) |
		del(.metadata.uid)' > ${WORKDIR}/${i}.yaml
done


for i in "${HIVE_TYPES[@]}"
do
	:
	# Only attempt to remove finalizers from types we found results for:
	if [ -s ${WORKDIR}/${i}.yaml ]
	then
		echo "Removing finalizers from all ${i}"
		oc get ${i}.hive.openshift.io -A -o json | jq '.items | .[] |
			.metadata.finalizers = null' | oc apply -f -
	fi
done
