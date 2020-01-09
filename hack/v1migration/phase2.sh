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

if [[ ! -d "$WORKDIR" ]]
then
    echo "Directory $WORKDIR does not exist."
    exit 1
fi


HIVE_TYPES=( checkpoints clusterdeployments clusterdeprovisionrequests clusterimagesets clusterprovisions clusterstates dnsendpoints dnszones hiveconfig selectorsyncidentityprovider selectorsyncset syncidentityprovider syncsetinstance syncset )
for i in "${HIVE_TYPES[@]}"
do
	:
	echo "Deleting all ${i}"
	oc delete ${i}.hive.openshift.io -A --all
done

oc delete customresourcedefinition.apiextensions.k8s.io/checkpoints.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/clusterdeployments.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/clusterdeprovisionrequests.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/clusterimagesets.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/clusterprovisions.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/clusterstates.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/dnsendpoints.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/dnszones.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/hiveconfigs.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/selectorsyncidentityproviders.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/selectorsyncsets.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/syncidentityproviders.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/syncsetinstances.hive.openshift.io
oc delete customresourcedefinition.apiextensions.k8s.io/syncsets.hive.openshift.io

