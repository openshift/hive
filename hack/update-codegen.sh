#!/bin/bash

set -x
set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(realpath $(dirname ${BASH_SOURCE[0]})/..)

GO111MODULE=on go install k8s.io/code-generator/cmd/deepcopy-gen@release-1.29

deepcopy-gen \
  -O zz_generated.deepcopy \
  --trim-path-prefix github.com/openshift/hive \
  --go-header-file "/dev/null" \
  --input-dirs github.com/openshift/hive/apis/hivecontracts/v1alpha1 \
  --input-dirs github.com/openshift/hive/apis/hiveinternal/v1alpha1 \
  --input-dirs github.com/openshift/hive/apis/hive/v1 \
  --input-dirs github.com/openshift/hive/apis/hive/v1/agent \
  --input-dirs github.com/openshift/hive/apis/hive/v1/aws \
  --input-dirs github.com/openshift/hive/apis/hive/v1/azure \
  --input-dirs github.com/openshift/hive/apis/hive/v1/baremetal \
  --input-dirs github.com/openshift/hive/apis/hive/v1/gcp \
  --input-dirs github.com/openshift/hive/apis/hive/v1/ibmcloud \
  --input-dirs github.com/openshift/hive/apis/hive/v1/metricsconfig \
  --input-dirs github.com/openshift/hive/apis/hive/v1/none \
  --input-dirs github.com/openshift/hive/apis/hive/v1/openstack \
  --input-dirs github.com/openshift/hive/apis/hive/v1/ovirt \
  --input-dirs github.com/openshift/hive/apis/hive/v1/vsphere
