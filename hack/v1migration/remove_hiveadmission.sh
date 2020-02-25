#!/bin/bash

set -e

echo "Deleting webhooks"
oc get validatingwebhookconfiguration -oname | \
  grep -E -e "\.hive\.openshift\.io$" | \
  xargs -i oc delete {}

echo "Deleting admission api service"
oc delete apiservice v1alpha1.admission.hive.openshift.io

echo "Deleting hiveadmission deployment"
oc delete -n hive deployment/hiveadmission