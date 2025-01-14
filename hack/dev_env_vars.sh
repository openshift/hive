export HIVE_ROOT=$(git rev-parse --show-toplevel)
export PATH=$HOME/.bin/go/bin:$PATH
export GOBIN=$HOME/go/bin
export PATH=$PATH:$GOBIN
export PATH=$GOPATH/bin:$PATH
export PATH=$HIVE_ROOT/.bin:$PATH
export CNI_PATH=$HIVE_ROOT/.bin/cni/bin
export IMG=localhost:5000/hive:latest
export KUBECONFIG=~/.kube/dev-hive.kubeconfig

