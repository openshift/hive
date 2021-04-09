#!/bin/bash

# exit on error
set -e

if [ $# -eq 0 ]; then
  echo "Usage: $(basename $0) ls <extra options to append to aws command>"
  echo "Usage: $(basename $0) sync <s3_log_dir> <extra options to append to aws command>"
  echo
  echo "ls lists the directories in s3 that contain install logs"
  echo "sync downloads the specified files in the specified s3_log_dir"
  echo
  echo "Examples:"
  echo "  # this lists all log directories in s3"
  echo "  $(basename $0) ls"
  echo
  echo "  # this downloads all logs in cluster1-6a85a345-cd5 to /tmp/logs"
  echo "  $(basename $0) sync cluster1-6a85a345-cd5 /tmp/logs"
  echo
  exit 5
fi

hiveconfig=$(oc get hiveconfig hive -o json)
if [ -z "$hiveconfig" ] ; then
  echo "Error: Unable to retrieve hiveconfig."
  exit 10
fi

bucket=$(echo "$hiveconfig" | jq -r '.spec.failedProvisionConfig.aws.bucket')
if [ -z "$bucket" -o "$bucket" == "null" ] ; then
  echo "Error: Unable to determine S3 bucket from hiveconfig."
  exit 10
fi

region=$(echo "$hiveconfig" | jq -r '.spec.failedProvisionConfig.aws.region')
if [ -z "$region" -o "$region" == "null" ] ; then
  echo "Error: Unable to determine AWS region from hiveconfig."
  exit 10
fi

secret_name=$(echo "$hiveconfig" | jq -r '.spec.failedProvisionConfig.aws.credentialsSecretRef.name')
if [ -z "$secret_name" -o "$secret_name" == "null" ] ; then
  echo "Error: Unable to determine AWS credentials secret name from hiveconfig."
  exit 10
fi

hive_ns=$(echo "$hiveconfig" | jq -r '.spec.targetNamespace')
if [ -z "$hive_ns" -o "$hive_ns" == "null" ] ; then
  hive_ns="hive" # default hive namespace to "hive"
fi

credentials_secret=$(oc get secret -n "$hive_ns" -o json "$secret_name")
if [ -z "$credentials_secret" -o "$credentials_secret" == "null" ] ; then
  echo "Error: Unable to retrieve credentials secret [$secret_name]."
  exit 10
fi

export AWS_ACCESS_KEY_ID=$(echo "$credentials_secret" | jq -r '.data.aws_access_key_id' | base64 -d)
if [ -z "$AWS_ACCESS_KEY_ID" -o "$AWS_ACCESS_KEY_ID" == "null" ] || [[ "$AWS_ACCESS_KEY_ID" != AKIA* ]] ; then
  echo "Error: Unable to retrieve aws_access_key_id from secret [$secret_name] in the [$hive_ns] namespace."
  exit 10
fi

export AWS_SECRET_ACCESS_KEY=$(echo "$credentials_secret" | jq -r '.data.aws_secret_access_key' | base64 -d)
if [ -z "$AWS_SECRET_ACCESS_KEY" -o "$AWS_SECRET_ACCESS_KEY" == "null" ]; then
  echo "Error: Unable to retrieve aws_secret_access_key from secret [$secret_name] in the [$hive_ns] namespace."
  exit 10
fi


subcmd=$1
shift # dump the sub-command from args so we can pass the rest of the args to the aws command

case $subcmd in
  ls) echo
      echo "Bucket [$bucket] contents:"
      aws s3 ls "s3://${bucket}/" $@
      echo
      ;;
  sync)
      s3_log_dir="$1"
      shift # dump the specified s3_log_dir so we can pass the rest of the args to the aws command
      if [ -z "s3_log_dir" ]; then
        echo "Error: s3_log_dir unspecified."
        exit 10
      fi
      aws s3 sync "s3://${bucket}/${s3_log_dir}" $@
      ;;
  *) echo "Error: unknown subcommand [$subcmd]."
     exit 20
     ;;
esac
