BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go')
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml

USE_BUILDAH ?= 1
ifeq ($(USE_BUILDAH), 1)
	BUILD_CMD = buildah build-using-dockerfile
else
	BUILD_CMD = docker build
endif


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

# Check import naming
verify-imports:
	@echo "Verifying import naming"
	$(foreach file,$(GOFILES), \
		$(shell $(BINDIR)/hiveutil verify-imports -c $(VERIFY_IMPORTS_CONFIG) $(file))\
	)


# Generate code
generate:
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build:
	# Put the '.' at the end so it works with both docker and buildah
	$(BUILD_CMD) -t ${IMG} .
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

	# If we built using buildah, then push it to the local docker-daemon
	# (so docker/'oc cluster up' can see/use it)
	$(if ifeq ($(USE_BUILDAH),1), buildah push ${IMG} docker-daemon:${IMG})

# Push the docker image
docker-push:
	docker push ${IMG}
