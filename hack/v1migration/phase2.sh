#!/bin/sh

set -e

if [ $# != 1 ]; then
	echo "USAGE: $0 [WORKDIR]"
	exit 1
fi

WORKDIR=$1
echo "Using workdir: $WORKDIR"

oc scale -n hive deployment.v1.apps/hive-operator --replicas=0
oc scale -n hive deployment.v1.apps/hive-controllers --replicas=0
oc scale -n hive deployment.v1.apps/hiveadmission --replicas=0

if [[ ! -d "$WORKDIR" ]]
then
    echo "Directory $WORKDIR does not exist."
    exit 1
fi

echo "WARNING: this script will delete all Hive custom resources and their definitions"
echo "It should only be used during migrations to the v1 API."
read -r -p "Do you wish to proceed? [y/N] " response
if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]
then
	exit 1
fi

readarray -t HIVE_TYPES <<< \
"$(oc get crd -o json | \
jq -r '.items[] | select(.spec.group="hive.openshift.io") | select(.spec.version="v1alpha1") | .spec.names.plural')"

for i in "${HIVE_TYPES[@]}"
do
	:
	echo "Deleting all ${i}"
	oc delete ${i}.hive.openshift.io -A --all
	echo "Deleting ${i}.hive.openshift.io CRD"
	oc delete customresourcedefinition.apiextensions.k8s.io/${i}.hive.openshift.io
done

