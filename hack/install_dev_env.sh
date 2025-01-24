#!/usr/bin/env bash

# Variables
HIVE_GO_VERSION="1.22"
ROOTLESSKIT_VERSION="v2.3.1"
CONTAINERD_VERSION="2.0.1"
SLIRP4NETNS_VERSION="v1.3.1"
KUSTOMIZE_VERSION="4.1.3"
NERDCTL_VERSION="2.0.2"
CFSSL_VERSION="1.6.5"
KIND_VERSION="v0.25.0"
BUILDKIT_VERSION="v0.18.2"
CNI_PLUGINS_VERSION="v1.6.2"
RUNC_VERSION="v1.2.3"
HIVE_ROOT="$(git rev-parse --show-toplevel)"
CNI_PATH="$HIVE_ROOT"/.tmp/_output/cni/bin

command_exists() {
 command -v "$1" >/dev/null 2>&1
}

# Requirements for running this script or not available in binary form
commands=("make" "awk" "sed" "git" "python3" "pip" "iptables")

for command in "${commands[@]}"; do
  if ! command_exists "$command"; then
    echo "Error: $command is not available"
    exit 1
  fi
done

export HIVE_ROOT

mkdir -p "$HIVE_ROOT"/.tmp/_output/bin
mkdir -p "$CNI_PATH"
export CNI_PATH
export PATH="${HIVE_ROOT}/.tmp/_output/bin:$PATH"

# Go setup
export GOPATH="${HIVE_ROOT}/.tmp/_output/go"
mkdir -p "$GOPATH"

export PATH="${GOPATH}/bin:$PATH"

GO_VERSION=$(go version 2>/dev/null)
CURRENT_VERSION=$(echo "$GO_VERSION" | awk '{print $3}' | sed 's/go//')

if [[ -z "$GO_VERSION" ]] || [[ "$CURRENT_VERSION" != "${HIVE_GO_VERSION}"* ]]; then
   curl -L https://go.dev/dl/go1.22.4.linux-amd64.tar.gz |\
   tar -C "$HIVE_ROOT"/.tmp/_output -xz
fi

go install github.com/golang/mock/mockgen@latest

# Build Hive
make


# Install Dependencies
if ! command_exists rootlesskit; then
  curl -L "https://github.com/rootless-containers/rootlesskit/releases/download/${ROOTLESSKIT_VERSION}/rootlesskit-x86_64.tar.gz" |\
  tar -C "${HIVE_ROOT}/.tmp/_output/bin" -xz rootlesskit
fi

# Install Containerd
if ! command_exists containerd; then
  curl -L "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-2.0.1-linux-amd64.tar.gz" |\
  tar -xz -C "${HIVE_ROOT}/.tmp/_output/bin" --strip-components=1 "bin/containerd" "bin/containerd-shim-runc-v2"
fi

# Install Slirp4netns
if ! command_exists slirp4netns; then
  curl -L "https://github.com/rootless-containers/slirp4netns/releases/download/${SLIRP4NETNS_VERSION}/slirp4netns-x86_64" -o "${HIVE_ROOT}/.tmp/_output/bin/slirp4netns"
  chmod +x "${HIVE_ROOT}/.tmp/_output/bin/slirp4netns"
fi

# Install Kustomize
if ! command_exists kustomize-4.1.3; then
 curl -L https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_amd64.tar.gz |\
 tar -xz -C "${HIVE_ROOT}/.tmp/_output/bin" kustomize && mv "$HIVE_ROOT"/.tmp/_output/bin/kustomize "$HIVE_ROOT"/.tmp/_output/bin/kustomize-4.1.3 
fi

# Install nerdctl
if ! command_exists nerdctl; then
  curl -L "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz" |\
  tar -xz -C "${HIVE_ROOT}/.tmp/_output/bin" containerd-rootless.sh containerd-rootless-setuptool.sh nerdctl
fi

# Install cfssl and cfssljson
if ! command_exists cfssl; then
  curl -L "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssl_${CFSSL_VERSION}_linux_amd64" -o "${HIVE_ROOT}/.tmp/_output/bin/cfssl"
  chmod +x "${HIVE_ROOT}/.tmp/_output/bin/cfssl"
fi

if ! command_exists cfssljson; then
  curl -L "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssljson_${CFSSL_VERSION}_linux_amd64" -o "${HIVE_ROOT}/.tmp/_output/bin/cfssljson"
  chmod +x "${HIVE_ROOT}/.tmp/_output/bin/cfssljson"
fi

# Install oc
if ! command_exists oc; then
  curl -L "https://github.com/okd-project/okd/releases/download/4.17.0-okd-scos.0/openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0.tar.gz" |\
  tar -xz -C "${HIVE_ROOT}/.tmp/_output/bin" "oc"
fi

# Install kubectl
if ! command_exists kubectl; then
  curl -L "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" -o "${HIVE_ROOT}/.tmp/_output/bin/kubectl"
  chmod +x "$HIVE_ROOT"/.tmp/_output/bin/kubectl
fi

# Install kind
if ! command_exists kind; then
  curl -L "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64" -o "${HIVE_ROOT}/.tmp/_output/bin/kind"
  chmod +x "$HIVE_ROOT"/.tmp/_output/bin/kind
fi

# Install buildkitd
if ! command_exists buildkitd; then
  curl -L "https://github.com/moby/buildkit/releases/download/${BUILDKIT_VERSION}/buildkit-${BUILDKIT_VERSION}.linux-amd64.tar.gz" |\
  tar -xz -C "${HIVE_ROOT}/.tmp/_output/bin" --strip-components=1 "bin/buildkitd" "bin/buildctl"
fi

# Install CNI plugins
if [ ! -f "$CNI_PATH/bridge" ]; then
  curl -L "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz" |\
  tar -xz -C "${CNI_PATH}"
fi

# Install runc
if ! command_exists runc; then
  curl -L "https://github.com/opencontainers/runc/releases/download/${RUNC_VERSION}/runc.amd64" -o "${HIVE_ROOT}/.tmp/_output/bin/runc"
  chmod +x "$HIVE_ROOT"/.tmp/_output/bin/runc
fi

echo "Checking system requirements for rootless containerd..."
CHECK_OUTPUT=$(containerd-rootless-setuptool.sh check)

if [[ "$CHECK_OUTPUT" != *"Requirements are satisfied"* ]]; then   echo "Error: Requirements are not satisfied. Exiting."
 exit 1
fi

CONTAINERD_SETUPTOOL_PATH="${HIVE_ROOT}/.tmp/_output/bin/containerd-rootless-setuptool.sh"

# Check if the line is already present (avoid duplicating it)
if ! grep -q "cni" $CONTAINERD_SETUPTOOL_PATH; then
  # Insert the flag if it's not present
  sed -i '/BUILDKITD_FLAG="--oci-cni-binary-dir=${HIVE_ROOT}\.tmp\/_output\/cni\/bin\/"/!s/\(BUILDKITD_FLAG="--oci-worker=true --oci-worker-rootless=true --containerd-worker=false\)/\1 --oci-cni-binary-dir=\/home\/daturece\/hive\/.tmp\/_output\/cni\/bin\//g' "$CONTAINERD_SETUPTOOL_PATH"
  echo "Flag added to setuptool.sh"
else
  echo "Flag already exists in setuptool.sh"
fi

# Install containerd in rootless mode, this will create a rootlesskit instance
containerd-rootless-setuptool.sh install
if [ $? -ne 0 ]; then
 echo "Error: Failed to install containerd in rootless mode."
 exit 1
fi

containerd-rootless-setuptool.sh install-buildkit
if [ $? -ne 0 ]; then
 echo "Error: Failed to install containerd in rootless mode."
 exit 1
fi

# Check if nerdctl is running
echo "Checking nerdctl version..."
NERDCTL_VERSION_OUTPUT=$(nerdctl --version 2>/dev/null)

if [[ "$NERDCTL_VERSION_OUTPUT" == *"nerdctl version 2.0.2"* ]]; then
 echo "nerdctl is running correctly: $NERDCTL_VERSION_OUTPUT"
else
 echo "Error: nerdctl is not running correctly."
 exit 1
fi
