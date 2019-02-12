BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go')
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml

# To use docker build, specify BUILD_CMD="docker build"
BUILD_CMD ?= imagebuilder

# Image URL to use all building/pushing image targets
IMG ?= hive-controller:latest

# Image to use when deploying
DEPLOY_IMAGE ?= registry.svc.ci.openshift.org/openshift/hive-v4.0:hive

# Look up distro name (e.g. Fedora)
DISTRO ?= $(shell if which lsb_release &> /dev/null; then lsb_release -si; else echo "Unknown"; fi)

# Default fedora to not using sudo since it's not needed
ifeq ($(DISTRO),Fedora)
	SUDO_CMD =
else # Other distros like RHEL 7 and CentOS 7 currently need sudo.
	SUDO_CMD = sudo
endif


all: fmt vet test build

# Run tests
.PHONY: test
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... ./contrib/... -coverprofile cover.out

.PHONY: test-integration
test-integration: generate
	go test ./test/integration/... -coverprofile cover.out

.PHONY: test-e2e
test-e2e:
	hack/e2e-test.sh

# Builds all of hive's binaries (including utils).
.PHONY: build
build: manager hiveutil hiveadmission


# Build manager binary
manager: generate
	go build -o bin/manager github.com/openshift/hive/cmd/manager

# Build hiveutil binary
hiveutil: generate
	go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil

# Build hiveadmission binary
hiveadmission:
	go build -o bin/hiveadmission github.com/openshift/hive/cmd/hiveadmission

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet
	go run ./cmd/manager/main.go --log-level=debug

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests
	kubectl apply -f manifests/
	kubectl apply -f config/crds
	mkdir -p config/overlays/deploy
	cp config/overlays/template/* config/overlays/deploy
	if [[ "`uname`" == "Darwin" ]]; then \
	    sed -i "" -e "s|IMAGE_REF|$(DEPLOY_IMAGE)|" config/overlays/deploy/image_patch.yaml; \
	else \
	    sed -i -e "s|IMAGE_REF|$(DEPLOY_IMAGE)|" config/overlays/deploy/image_patch.yaml; \
	fi
	kustomize build config/overlays/deploy | kubectl apply -f -
	rm -rf config/overlays/deploy
	kubectl apply -f config/rbac/hiveadmission_rbac_role.yaml -f config/rbac/hiveadmission_rbac_role_binding.yaml

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy-sd-dev
deploy-sd-dev: manifests
	kubectl apply -f config/crds
	kustomize build config/overlays/sd-dev | kubectl apply -f -
	kubectl apply -f config/rbac/hiveadmission_rbac_role.yaml -f config/rbac/hiveadmission_rbac_role_binding.yaml

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
.PHONY: fmt
fmt:
	gofmt -w -s $(SRC_DIRS)

# Run go vet against code
.PHONY: vet
vet:
	go vet ./pkg/... ./cmd/... ./contrib/...

# Run verification tests
.PHONY: verify
verify: verify-imports verify-gofmt verify-lint verify-go-vet

# Check import naming
.PHONY: verify-imports
verify-imports: hiveutil
	@echo "Verifying import naming"
	@sh -c \
	  'for file in $(GOFILES) ; do \
	     $(BINDIR)/hiveutil verify-imports -c $(VERIFY_IMPORTS_CONFIG) $$file || exit 1 ; \
	   done'

# Check import naming
.PHONY: verify-lint
verify-lint:
	@echo Verifying golint
	@sh -c \
	  'for file in $(GOFILES) ; do \
	     golint --set_exit_status $$file || exit 1 ; \
	   done'

.PHONY: verify-gofmt
verify-gofmt:
	@echo Verifying gofmt
	@gofmt -l -s $(SRC_DIRS)>.out 2>&1 || true
	@[ ! -s .out ] || \
	  (echo && echo "*** Please run 'make fmt' in order to fix the following:" && \
	  cat .out && echo && rm .out && false)
	@rm .out

.PHONY: verify-go-vet
verify-go-vet: generate
	@echo Verifying go vet
	@go vet ./cmd/... ./contrib/... $(go list ./pkg/... | grep -v _generated)

# Generate code
.PHONY: generate
generate:
	go generate ./pkg/... ./cmd/...

# Build the docker image
.PHONY: docker-build
docker-build: generate
	$(BUILD_CMD) -t ${IMG} .

# Push the docker image
.PHONY: docker-push
docker-push:
	$(DOCKER_CMD) push ${IMG}

# Build the image with buildah
.PHONY: buildah-build
buildah-build: generate
	$(SUDO_CMD) buildah bud --tag ${IMG} .

# Push the buildah image
.PHONY: buildah-push
buildah-push: buildah-build
	$(SUDO_CMD) buildah push ${IMG}

install-federation:
	./hack/install-federation.sh
