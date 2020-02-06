verify_scaledown() {
  local deployment_name=${1:?must specify a deployment name}
  echo "Verifying that $deployment_name has been scaled down"
  oc rollout status -n hive "deployment.v1.apps/$deployment_name" -w
  if [[ "$(oc get -n hive "deployment.v1.apps/$deployment_name" -o jsonpath='{.spec.replicas}')" != "0" ]]
  then
    echo "$deployment_name has not been scaled down to 0"
    exit 1
  fi
}

verify_all_scaled_down() {
  verify_scaledown "hive-operator"
  verify_scaledown "hive-controllers"
}
