#!/bin/bash

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

case "${2}" in
"--dry-run" )
  dry_run="--dry-run"
  ;;
"" )
  # When doing a dry run, allow the script to continue after errors. Operations such as deleting a CRD are likely
  # to fail since the CRs are not being deleted in a dry run.
  set -e
  ;;
* )
  echo "Only valid flag is \"--dry-run\""
  exit 1
esac

if [[ -z $dry_run ]]
then
  source "$(dirname $0)/verify_scaledown.sh"
  verify_all_scaled_down
fi

mkdir -p "$WORKDIR"

if [[ -z $dry_run && \
      "$(ls "$WORKDIR" 2>/dev/null)" ]]
then
  echo "WARNING: workdir ($WORKDIR) is not empty."
  echo "Files in workdir may be overwritten."
  read -r -p "Do you wish to proceed? [y/N] " response
  if [[ ! "$response" =~ ^([yY]([eE][sS])?)$ ]]
  then
    exit 1
  fi
fi

HIVE_TYPES=( checkpoints clusterdeployments clusterdeprovisionrequests clusterimagesets clusterprovisions clusterstates dnsendpoints dnszones hiveconfigs selectorsyncidentityproviders selectorsyncsets syncidentityproviders syncsetinstances syncsets )

for t in "${HIVE_TYPES[@]}"
do
	echo "Storing all ${t} in ${WORKDIR}/${t}.json"
	oc get "${t}.hive.openshift.io" -A -o json | jq .items[] > "${WORKDIR}/${t}.json"
done

bin/hiveutil v1migration save-owner-refs --work-dir "${WORKDIR}"

if [[ -z $dry_run ]]
then
  echo "WARNING: this script will delete all Hive custom resources and their definitions"
  echo "It should only be used during migrations to the v1 API."
  read -r -p "Do you wish to proceed? [y/N] " response
  if [[ ! "$response" =~ ^([yY]([eE][sS])?)$ ]]
  then
    exit 1
  fi
fi

if [[ -z $dry_run ]]
then
  echo "Deleting install jobs"
  oc delete jobs -l "hive.openshift.io/install=true" -A
fi

if [[ -z $dry_run ]]
then
  echo "Deleting imageset jobs"
  oc delete jobs -l "hive.openshift.io/imageset=true" -A
fi

for t in "${HIVE_TYPES[@]}"
do
  echo "Deleting ${t} objects"
  bin/hiveutil v1migration delete-objects "${WORKDIR}/${t}.json" $dry_run

  if [[ -z $dry_run ]]
  then
    for i in 1 2 3 4 5
    do
      if [[ "$(oc get "${t}.hive.openshift.io" -A -o name | wc -l)" -eq 0 ]]; then break; fi
      if (( i == 5 ))
      then
        echo "There are still remaining ${t}"
        exit 1
      fi
      echo "Waiting for delete of ${t} to complete..."
      sleep 1
    done
  fi

	echo "Deleting ${t}.hive.openshift.io CRD"
	if [[ -z $dry_run ]]
	then
	  oc delete "customresourcedefinition.apiextensions.k8s.io/${t}.hive.openshift.io" $dry_run
	fi
done

echo "Deleting webhooks"
if [[ -z $dry_run ]]
then
  oc get validatingwebhookconfiguration -oname | \
    grep -E -e "\.hive\.openshift\.io$" | \
    xargs -i oc delete {} $dry_run
fi

echo "Deleting admission api service"
if [[ -z $dry_run ]]
then
  oc delete apiservice v1alpha1.admission.hive.openshift.io $dry_run
fi