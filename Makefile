BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go' | grep -v bindata)
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml

FIRST_GOPATH:=$(firstword $(subst :, ,$(shell go env GOPATH)))
GOLANGCI_LINT_BIN=$(FIRST_GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION=v1.17.1

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

.PHONY: default
default: all

.PHONY: all
all: fmt vet generate verify test build

.PHONY: vendor
vendor:
	dep ensure -v

# Run tests
.PHONY: test
test: generate fmt vet crd lint
	go test ./pkg/... ./cmd/... ./contrib/... -coverprofile cover.out

.PHONY: test-integration
test-integration: generate
	go test ./test/integration/...

.PHONY: test-e2e
test-e2e:
	hack/e2e-test.sh

.PHONY: test-e2e-postdeploy
test-e2e-postdeploy:
	go test -timeout 0 ./test/e2e/postdeploy/...

.PHONY: test-e2e-postinstall
test-e2e-postinstall:
	go test -timeout 0 ./test/e2e/postinstall/...

# Builds all of hive's binaries (including utils).
.PHONY: build
build: $(GOPATH)/bin/mockgen manager hiveutil hiveadmission operator


# Build manager binary
manager: generate
	go build -o bin/manager github.com/openshift/hive/cmd/manager

operator: generate
	go build -o bin/hive-operator github.com/openshift/hive/cmd/operator

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

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run-operator
run-operator: generate fmt vet
	go run ./cmd/operator/main.go --log-level=debug

# Install CRDs into a cluster
.PHONY: install
install: crd
	oc apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests install generate
	# Deploy the operator manifests:
	mkdir -p overlays/deploy
	cp overlays/template/kustomization.yaml overlays/deploy
	cd overlays/deploy && kustomize edit set image registry.svc.ci.openshift.org/openshift/hive-v4.0:hive=${DEPLOY_IMAGE}
	kustomize build overlays/deploy | oc apply -f -
	rm -rf overlays/deploy

# Update the manifest directory of artifacts OLM will deploy. Copies files in from
# the locations kubebuilder generates them.
.PHONY: manifests
manifests: crd

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy-sd-dev
deploy-sd-dev: crd
	oc apply -f config/crds
	kustomize build overlays/sd-dev | oc apply -f -

# Generate CRD yaml from our api types:
.PHONY: crd
crd:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd

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
verify: verify-generated verify-imports verify-gofmt verify-lint verify-go-vet

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

.PHONY: verify-generated
verify-generated:
	hack/verify-generated.sh

# Generate code
.PHONY: generate
generate: $(GOPATH)/bin/mockgen
	go generate ./pkg/... ./cmd/...
	hack/update-bindata.sh

# Build the docker image
.PHONY: docker-build
docker-build:
	$(BUILD_CMD) -t ${IMG} .

# Push the docker image
.PHONY: docker-push
docker-push:
	$(DOCKER_CMD) push ${IMG}

# Build the image with buildah
.PHONY: buildah-build
buildah-build:
	$(SUDO_CMD) buildah bud --tag ${IMG} .

# Build the code locally and build+push an image with the local binaries rather than doing a full in-container compile.
.PHONY: buildah-dev-push
buildah-dev-push: build
	$(SUDO_CMD) buildah bud -f Dockerfile.dev --tag ${IMG} .
	$(SUDO_CMD) buildah push ${IMG}

# Push the buildah image
.PHONY: buildah-push
buildah-push: buildah-build
	$(SUDO_CMD) buildah push ${IMG}

$(GOPATH)/bin/mockgen:
	go get -u github.com/golang/mock/mockgen/...

.PHONY: clean ## Remove all build artifacts
clean:
	rm -rf $(BINDIR)

$(GOLANGCI_LINT_BIN):
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh \
		| sed -e '/install -d/d' \
| sh -s -- -b $(FIRST_GOPATH)/bin $(GOLANGCI_LINT_VERSION)

# Run golangci-lint against code
# TODO replace verify (except verify-generated), vet, fmt targets with lint as it covers all of it
.PHONY: lint
lint: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run -c ./golangci.yml ./pkg/... ./cmd/... ./contrib/...
