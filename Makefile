BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go' | grep -v bindata | grep -v generated)
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml

# To use docker build, specify BUILD_CMD="docker build"
BUILD_CMD ?= imagebuilder

DOCKER_CMD ?= docker

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

# set the cache directory to an accessible location
ifeq ($(XDG_CACHE_HOME),)
	export XDG_CACHE_HOME:=/tmp
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
	go test -timeout 0 -count=1 ./test/e2e/postdeploy/...

.PHONY: test-e2e-postinstall
test-e2e-postinstall:
	go test -timeout 0 -count=1 ./test/e2e/postinstall/...

# Builds all of hive's binaries (including utils).
.PHONY: build
build: manager hiveutil hiveadmission operator hive-apiserver


# Build manager binary
.PHONY: manager
manager: generate
	go build -o bin/manager github.com/openshift/hive/cmd/manager

.PHONY: operator
operator: generate
	go build -o bin/hive-operator github.com/openshift/hive/cmd/operator

# Build hiveutil binary
.PHONY: hiveutil
hiveutil: generate
	go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil

# Build hiveadmission binary
.PHONY: hiveadmission
hiveadmission:
	go build -o bin/hiveadmission github.com/openshift/hive/cmd/hiveadmission

# Build v1alpha1 aggregated API server
.PHONY: hive-apiserver
hive-apiserver:
	go build -o bin/hive-apiserver github.com/openshift/hive/cmd/hive-apiserver

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

# Generate CRD yaml from our api types:
.PHONY: crd
crd:
	# The apis-path is explicitly specified so that CRDs are not created for v1alpha1
	go run tools/vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd --apis-path=pkg/apis/hive/v1

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
	     if [[ $$file == "pkg/api/validation"* ]]; then continue; fi; \
	     if [[ $$file == "pkg/hive/apis/hive/hiveconfig_types.go" ]]; then continue; fi; \
	     if [[ $$file == "pkg/hive/apis/hive/hiveconversion/"* ]]; then continue; fi; \
	     if [[ $$file == "pkg/hive/apiserver/registry/"* ]]; then continue; fi; \
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
generate:
	go generate ./pkg/... ./cmd/...
	hack/update-bindata.sh

# Build the docker image
.PHONY: docker-build
docker-build:
	$(BUILD_CMD) -t ${IMG} .

# Build the docker image
.PHONY: docker-dev-push
docker-dev-push: manifests generate build
	$(DOCKER_CMD) build -t ${IMG} -f Dockerfile.dev .
	$(DOCKER_CMD) push ${IMG}

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

.PHONY: clean ## Remove all build artifacts
clean:
	rm -rf $(BINDIR)

# Run golangci-lint against code
# TODO replace verify (except verify-generated), vet, fmt targets with lint as it covers all of it
.PHONY: lint
lint:
	golangci-lint run -c ./golangci.yml ./pkg/... ./cmd/... ./contrib/...

# Build the build image so that it can be used locally for performing builds.
build-build-image: build/build-image/Dockerfile
	$(BUILD_CMD) -t "hive-build:latest" -f build/build-image/Dockerfile .
