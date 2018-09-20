
# Image URL to use all building/pushing image targets
IMG ?= controller:latest

all: test build

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Builds all of hive's binaries (including utils).
.PHONY: build
build: manager hiveutil


# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/openshift/hive/cmd/manager

# Build hiveutil binary
hiveutil: fmt vet
	go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/... ./contrib/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/... ./contrib/...

# Generate code
generate:
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
