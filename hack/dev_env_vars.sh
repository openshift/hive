#!/usr/bin/env bash

HIVE_ROOT="$(git rev-parse --show-toplevel)"
export HIVE_ROOT
export GOPATH="${HIVE_ROOT}/.tmp/_output/go"
export PATH=$HIVE_ROOT/.tmp/_output/bin:$HIVE_ROOT/.tmp/_output/go/bin:${GOPATH}/bin:$PATH:$PATH
export CNI_PATH=$HIVE_ROOT/.tmp/_output/cni/bin
export IMG=localhost:5000/hive:latest
export KUBECONFIG=$HIVE_ROOT/.tmp/_output/dev-hive.kubeconfig
export HIVE_OPERATOR_NS=dev-hive
export HIVE_SKIP_LEADER_ELECTION=1
export METRICS_CONFIG_FILE=metrics.json
export HIVE_NS=dev-hive
export HIVE_MACHINEPOOL_POD_NAME="hive-machinepool-0"
export HIVE_CLUSTERSYNC_POD_NAME="hive-clustersync-0"
export GOROOT=$HIVE_ROOT/.tmp/_output/go