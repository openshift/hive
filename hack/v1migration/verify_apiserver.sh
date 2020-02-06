verify_apiserver() {
  if [[ $(oc get apiservice v1alpha1.hive.openshift.io -o json | jq '.status.conditions[]? | select(.type=="Available") | .status=="True"') != "true" ]]
  then
    echo "Hive aggregated API server is not available"
    exit 1
  fi
}
