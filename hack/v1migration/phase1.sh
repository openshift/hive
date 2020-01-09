#!/bin/sh

set -e

if [ $# != 1 ]; then
	echo "USAGE: $0 [WORKDIR]"
	exit 1
fi

WORKDIR=$1
echo "Using workdir: $WORKDIR"

oc scale -n hive deployment.v1.apps/hive-operator --replicas=0

# TODO: wait for operator scaledown

oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0

# TODO: wait until no hive pods running

mkdir -p $WORKDIR/

readarray -t HIVE_TYPES <<< \
"$(oc get crd -o json | \
jq -r '.items[] | select(.spec.group="hive.openshift.io") | select(.spec.version="v1alpha1") | .spec.names.plural')"

for i in "${HIVE_TYPES[@]}"
do
	:
	echo "Storing all ${i} in ${WORKDIR}/${i}.json"
	oc get ${i}.hive.openshift.io -A -o json | jq '.items[] |
		del(.metadata.annotations."kubectl.kubernetes.io/last-applied-configuration")' > ${WORKDIR}/${i}.json
done


for i in "${HIVE_TYPES[@]}"
do
	:
	# Only attempt to remove finalizers from types we found results for:
	if [ -s ${WORKDIR}/${i}.json ]
	then
		echo "Removing finalizers from all ${i}"
		oc get ${i}.hive.openshift.io -A -o json | jq '.items[] |
			.metadata.finalizers = null' | oc apply -f -
	fi
done
