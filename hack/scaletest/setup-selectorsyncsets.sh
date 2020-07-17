#!/bin/bash

set -e

SYNCSET_COUNT=60

for (( i=0; i<=${SYNCSET_COUNT}; i++ ))
do
	cluster_name="c${i}"
	ns="ns${i}"
	echo "Creating SelectorSyncSet ${i}"
	oc process -f hack/scaletest/syncset-template.yaml RESOURCE_INDEX=${i} | oc apply -f -
done

