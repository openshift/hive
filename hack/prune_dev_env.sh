#!/usr/bin/env bash

HIVE_ROOT="$(git rev-parse --show-toplevel)"
export HIVE_ROOT
export PATH=$HIVE_ROOT/.tmp/_output/bin:$PATH

containerd_pids=$(nerdctl ps -q)

# Stop containers
for pid in $containerd_pids; do
  nerdctl stop "$pid"
  nerdctl remove "$pid"
done

# clear cache
nerdctl system prune -a -f

# stop & uninstall rootless buildkitd
containerd-rootless-setuptool.sh uninstall-buildkit
rootlesskit rm -rf ~/.local/share/buildkit

# stop & uninstall rootlesscontainerd
containerd-rootless-setuptool.sh uninstall
rootlesskit rm -rf ~/.local/share/containerd

# remove dirs create by scripts
go clean -cache -modcache
rm -rf "$HIVE_ROOT"/.tmp "$HIVE_ROOT"/hiveadmission-certs