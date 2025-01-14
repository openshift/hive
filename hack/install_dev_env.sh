#!/usr/bin/env bash

export HIVE_ROOT=$(git rev-parse --show-toplevel)

mkdir -p $HIVE_ROOT/.bin

GO_VERSION=$(go version 2>/dev/null)

command_exists() {
 command -v "$1" >/dev/null 2>&1
}

if ! command_exists make; then
 sudo dnf install -y make
else
 echo "Make is already installed."
fi

if ! command_exists gcc; then
 sudo dnf install -y gcc
else
 echo "GCC is already installed."
fi


if [ $? -eq 0 ]; then

   CURRENT_VERSION=$(echo "$GO_VERSION" | awk '{print $3}' | sed 's/go//')

   # Compare the current Go version with 1.22
   if [[ "$CURRENT_VERSION" == "1.22"* ]]; then
           echo "Go version 1.22 or higher is already installed."
   else
           echo "Go version is $CURRENT_VERSION, installing Go according to go.mod"
           mkdir -p $HOME/.bin
           curl -Lo go1.22.0.linux-amd64.tar.gz https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
           tar -C $HOME/.bin -xzf go1.22.0.linux-amd64.tar.gz
   fi
else
   # If Go is not installed, install Go 1.22
   echo "Go is not installed. Installing Go version 1.22"
   mkdir -p $HOME/.bin
   curl -Lo go1.22.0.linux-amd64.tar.gz https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
   tar -C $HOME/.bin -xzf go1.22.0.linux-amd64.tar.gz
fi

# Source Go env variables
source hack/dev_env_vars.sh

go install github.com/golang/mock/mockgen@latest

make

# Install Rootlesskit
if ! command_exists rootlesskit; then
 curl -Lo rootlesskit-x86_64.tar.gz https://github.com/rootless-containers/rootlesskit/releases/download/v2.3.1/rootlesskit-x86_64.tar.gz
 tar -xzf rootlesskit-x86_64.tar.gz --one-top-level && rm rootlesskit-x86_64.tar.gz
 mv rootlesskit-x86_64/rootlesskit $HIVE_ROOT/.bin
 rm -r rootlesskit-x86_64
 echo "Rootlesskit installed."
else
 echo "Rootlesskit is already installed."
fi

# Install Containerd
if ! command_exists containerd; then
 curl -Lo containerd-2.0.1-linux-amd64.tar.gz https://github.com/containerd/containerd/releases/download/v2.0.1/containerd-2.0.1-linux-amd64.tar.gz
 tar -xzf containerd-2.0.1-linux-amd64.tar.gz --one-top-level && rm containerd-2.0.1-linux-amd64.tar.gz
 mv containerd-2.0.1-linux-amd64/bin/containerd $HIVE_ROOT/.bin/
 mv containerd-2.0.1-linux-amd64/bin/containerd-shim-runc-v2 $HIVE_ROOT/.bin/
 rm -r containerd-2.0.1-linux-amd64
 echo "Containerd installed."
else
 echo "Containerd is already installed."
fi

# Install Slirp4netns
if ! command_exists slirp4netns; then
 curl -Lo slirp4netns-x86_64 https://github.com/rootless-containers/slirp4netns/releases/download/v1.3.1/slirp4netns-x86_64
 chmod +x slirp4netns-x86_64
 mv ./slirp4netns-x86_64 $HIVE_ROOT/.bin/slirp4netns
 echo "Slirp4netns installed."
else
 echo "Slirp4netns is already installed."
fi

# Install Kustomize
if ! command_exists kustomize-4.1.3; then
 curl -Lo kustomize_v4.1.3_linux_amd64.tar.gz https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv4.1.3/kustomize_v4.1.3_linux_amd64.tar.gz
 tar -xvf kustomize_v4.1.3_linux_amd64.tar.gz && rm kustomize_v4.1.3_linux_amd64.tar.gz
 mv ./kustomize $HIVE_ROOT/.bin/kustomize-4.1.3
else
 echo "Kustomize is already installed."
fi

# Install nerdctl
if ! command_exists nerdctl; then
 curl -Lo nerdctl-2.0.2-linux-amd64.tar.gz https://github.com/containerd/nerdctl/releases/download/v2.0.2/nerdctl-2.0.2-linux-amd64.tar.gz
 tar -xvf nerdctl-2.0.2-linux-amd64.tar.gz --one-top-level && rm nerdctl-2.0.2-linux-amd64.tar.gz
 mv nerdctl-2.0.2-linux-amd64/containerd-rootless.sh $HIVE_ROOT/.bin/
 mv nerdctl-2.0.2-linux-amd64/containerd-rootless-setuptool.sh $HIVE_ROOT/.bin/
 mv nerdctl-2.0.2-linux-amd64/nerdctl $HIVE_ROOT/.bin/
 rm -r nerdctl-2.0.2-linux-amd64*
 echo "nerdctl installed."
else
 echo "nerdctl is already installed."
fi

# Install cfssl & cfssljson
if ! command_exists cfssl; then
 curl -Lo cfssl_1.6.5_linux_amd64 https://github.com/cloudflare/cfssl/releases/download/v1.6.5/cfssl_1.6.5_linux_amd64
 chmod +x cfssl_1.6.5_linux_amd64
 mv ./cfssl_1.6.5_linux_amd64 $HIVE_ROOT/.bin/cfssl
 curl -Lo cfssljson_1.6.5_linux_amd64 https://github.com/cloudflare/cfssl/releases/download/v1.6.5/cfssljson_1.6.5_linux_amd64
 chmod +x cfssljson_1.6.5_linux_amd64
 mv ./cfssljson_1.6.5_linux_amd64 $HIVE_ROOT/.bin/cfssljson
 echo "cfssl installed."
else
 echo "cfssl is already installed."
fi

# Install oc
if ! command_exists oc; then
 curl -Lo openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0.tar.gz https://github.com/okd-project/okd/releases/download/4.17.0-okd-scos.0/openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0.tar.gz
 tar -xvf openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0.tar.gz --one-top-level && rm openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0.tar.gz
 chmod +x openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0/oc
 mv openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0/oc $HIVE_ROOT/.bin/
 echo "kubectl installed."
 rm -r openshift-client-linux-amd64-rhel8-4.17.0-okd-scos.0*
else
 echo "oc is already installed."
fi

# Install kubectl
if ! command_exists kubectl; then
 curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
 chmod +x kubectl
 mv kubectl $HIVE_ROOT/.bin/
 echo "kubectl installed."+
else
 echo "kubectl is already installed."
fi

# Install kind
if ! command_exists kind; then
 curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.25.0/kind-linux-amd64
 chmod +x ./kind
 mv ./kind $HIVE_ROOT/.bin/kind
 echo "kind installed"
else
 echo "kind is already installed."
fi

# Install buildkitd
if ! command_exists buildkitd; then
 curl -Lo buildkit-v0.18.2.linux-amd64.tar.gz https://github.com/moby/buildkit/releases/download/v0.18.2/buildkit-v0.18.2.linux-amd64.tar.gz
 tar -xf buildkit-v0.18.2.linux-amd64.tar.gz --one-top-level && rm buildkit-v0.18.2.linux-amd64.tar.gz
 mv buildkit-v0.18.2.linux-amd64/bin/buildkitd $HIVE_ROOT/.bin/
 mv buildkit-v0.18.2.linux-amd64/bin/buildctl $HIVE_ROOT/.bin/
 echo "buildkitd installed."
 rm -r buildkit-v0.18.2.linux-amd64*
else
 echo "buildkitd is already installed."
fi

# Install CNI plugins
if [ ! -d "$HIVE_ROOT/.bin/cni/bin" ]; then
 curl -Lo cni-plugins-linux-amd64-v1.6.0.tgz https://github.com/containernetworking/plugins/releases/download/v1.6.0/cni-plugins-linux-amd64-v1.6.0.tgz
 tar -xf cni-plugins-linux-amd64-v1.6.0.tgz --one-top-level && rm cni-plugins-linux-amd64-v1.6.0.tgz
 mkdir -p $HIVE_ROOT/.bin/cni/bin
 mv cni-plugins-linux-amd64-v1.6.0/* $HIVE_ROOT/.bin/cni/bin/
 echo "CNI plugins installed."
 rm -r cni-plugins-linux-amd64-v1.6.0*
else
 echo "CNI plugins are already installed."
fi

# Install runc
if ! command_exists runc; then
 curl -Lo runc.amd64 https://github.com/opencontainers/runc/releases/download/v1.2.3/runc.amd64
 chmod +x runc.amd64
 mv ./runc.amd64 $HIVE_ROOT/.bin/runc
 echo "runc installed."
else
 echo "Runc is already installed."
fi

# Check requirements

export CNI_PATH=$HIVE_ROOT/.bin/cni/bin/

echo "Checking system requirements for rootless containerd..."
CHECK_OUTPUT=$(containerd-rootless-setuptool.sh check)

if [[ "$CHECK_OUTPUT" != *"Requirements are satisfied"* ]]; then   echo "Error: Requirements are not satisfied. Exiting."
 exit 1
fi

# Install containerd in rootless mode, this will createa rootlesskit instance
containerd-rootless-setuptool.sh install
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

echo "Setup complete."

