.PHONY: all
all: vendor update test build

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	lib/tmp.mk \
	targets/openshift/controller-gen.mk \
	targets/openshift/yq.mk \
	targets/openshift/bindata.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
)

DOCKER_CMD ?= docker

# Namespace hive-operator will run:
HIVE_OPERATOR_NS ?= hive

# Namespace hive-controllers/hiveadmission/etc will run:
HIVE_NS ?= hive

# Log level that should be used when running hive from source, or with make deploy.
LOG_LEVEL ?= debug

# Image URL to use all building/pushing image targets
IMG ?= hive-controller:latest

# Image to use when deploying
DEPLOY_IMAGE ?= registry.svc.ci.openshift.org/openshift/hive-v4.0:hive

GO_PACKAGES :=$(addsuffix ...,$(addprefix ./,$(filter-out vendor/,$(filter-out v1alpha1apiserver/,$(wildcard */)))))
GO_BUILD_PACKAGES :=./cmd/... ./contrib/cmd/hiveutil
GO_BUILD_BINDIR :=bin
# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/... ./contrib/...

ifeq "$(GO_MOD_FLAGS)" "-mod=vendor"
	ifeq "$(GOFLAGS)" ""
		GOFLAGS_FOR_GENERATE ?= GOFLAGS=-mod=vendor
	else
		GOFLAGS_FOR_GENERATE ?= GOFLAGS=-mod=vendor,$(GOFLAGS)
	endif
endif

# Look up distro name (e.g. Fedora)
DISTRO ?= $(shell if which lsb_release &> /dev/null; then lsb_release -si; else echo "Unknown"; fi)

# Default fedora to not using sudo since it's not needed
ifeq ($(DISTRO),Fedora)
	SUDO_CMD =
else # Other distros like RHEL 7 and CentOS 7 currently need sudo.
	SUDO_CMD = sudo
endif

BINDATA_INPUTS :=./config/apiserver/... ./config/hiveadmission/... ./config/controllers/... ./config/rbac/... ./config/crds/... ./config/configmaps/...
$(call add-bindata,operator,$(BINDATA_INPUTS),,assets,pkg/operator/assets/bindata.go)

$(call build-image,hive,$(IMG),./Dockerfile,.)
$(call build-image,hive-dev,$(IMG),./Dockerfile.dev,.)
$(call build-image,hive-build,"hive-build:latest",./build/build-image/Dockerfile,.)

clean:
	rm -rf $(GO_BUILD_BINDIR)

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor
	$(MAKE) -C v1alpha1apiserver vendor

# Update the manifest directory of artifacts OLM will deploy. Copies files in from
# the locations kubebuilder generates them.
.PHONY: manifests
manifests: crd

# controller-gen is adding a yaml break (---) at the beginning of each file. OLM does not like this break.
# We use yq to strip out the yaml break by having yq replace each file with yq's formatting.
# This also removes the spec.validation.openAPIV3Schema.type field which OpenShift 3.11 does not like.
# $1 - CRD file
define strip-yaml-break
	@$(YQ) d -i $(1) spec.validation.openAPIV3Schema.type

endef

# Generate CRD yaml from our api types:
.PHONY: crd
crd: ensure-controller-gen ensure-yq
	rm -rf ./config/crds
	'$(CONTROLLER_GEN)' crd paths=./pkg/apis/hive/v1 output:dir=./config/crds
	@echo Stripping yaml breaks from CRD files
	$(foreach p,$(wildcard ./config/crds/*.yaml),$(call strip-yaml-break,$(p)))
update: crd

.PHONY: verify-crd
verify-crd: ensure-controller-gen ensure-yq
	./hack/verify-crd.sh
verify: verify-crd

.PHONY: test-integration
test-integration: generate
	go test $(GO_MOD_FLAGS) ./test/integration/...

.PHONY: test-e2e
test-e2e:
	hack/e2e-test.sh

.PHONY: test-e2e-postdeploy
test-e2e-postdeploy:
	go test $(GO_MOD_FLAGS) -v -timeout 0 -count=1 ./test/e2e/postdeploy/...

.PHONY: test-e2e-postinstall
test-e2e-postinstall:
	go test $(GO_MOD_FLAGS) -v -timeout 0 -count=1 ./test/e2e/postinstall/...

.PHONY: test-e2e-destroycluster
test-e2e-destroycluster:
	go test $(GO_MOD_FLAGS) -v -timeout 0 -count=1 ./test/e2e/destroycluster/...

.PHONY: test-e2e-uninstallhive
test-e2e-uninstallhive:
	go test $(GO_MOD_FLAGS) -v -timeout 0 -count=1 ./test/e2e/uninstallhive/...

.PHONY: build-apiserver
build-apiserver:
	$(MAKE) -C v1alpha1apiserver build
build: build-apiserver

# Run against the configured cluster in ~/.kube/config
run: build
	./bin/manager --log-level=${LOG_LEVEL}

# Run against the configured cluster in ~/.kube/config
run-operator: build
	./bin/operator --log-level=${LOG_LEVEL}

# Install CRDs into a cluster
install: crd
	oc apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: install
	# Deploy the operator manifests:
	oc create namespace ${HIVE_OPERATOR_NS} || true
	mkdir -p overlays/deploy
	cp overlays/template/kustomization.yaml overlays/deploy
	cd overlays/deploy && kustomize edit set image registry.svc.ci.openshift.org/openshift/hive-v4.0:hive=${DEPLOY_IMAGE} && kustomize edit set namespace ${HIVE_OPERATOR_NS}
	kustomize build overlays/deploy | oc apply -f -
	rm -rf overlays/deploy
	# Create a default basic HiveConfig so the operator will deploy Hive
	oc process --local=true -p HIVE_NS=${HIVE_NS} -p LOG_LEVEL=${LOG_LEVEL} -f config/templates/hiveconfig.yaml | oc apply -f -

verify-codegen:
	bash -x hack/verify-codegen.sh
verify: verify-codegen

update-codegen:
	hack/update-codegen.sh
update: update-codegen

# Check import naming
.PHONY: verify-imports
verify-imports: build
	@echo "Verifying import naming"
	@sh -c \
	  'for file in $(GOFILES) ; do \
	     $(BINDIR)/hiveutil verify-imports -c $(VERIFY_IMPORTS_CONFIG) $$file || exit 1 ; \
	   done'
verify: verify-imports

# Check lint
.PHONY: verify-lint
verify-lint: install-tools
	@echo Verifying golint
	@sh -c \
	  'for file in $(GOFILES) ; do \
	     golint --set_exit_status $$file || exit 1 ; \
	   done'
verify: verify-lint

# Generate code
.PHONY: generate
generate: install-tools
	$(GOFLAGS_FOR_GENERATE) go generate ./pkg/... ./cmd/...
update: generate

# Build the image using docker
.PHONY: docker-build
docker-build:
	@echo "*** DEPRECATED: Use the image-hive target instead ***"
	$(DOCKER_CMD) build -t ${IMG} .

# Build the dev image using docker
.PHONY: docker-dev-build
docker-dev-build: build
	@echo "*** DEPRECATED: Use the image-hive-dev target instead ***"
	$(DOCKER_CMD) build -t ${IMG} -f Dockerfile.dev .

# Push the image using docker
.PHONY: docker-push
docker-push:
	$(DOCKER_CMD) push ${IMG}

# Build and push the dev image
.PHONY: docker-dev-push
docker-dev-push: build image-hive-dev docker-push

# Push the image using buildah
.PHONY: buildah-push
buildah-push:
	$(SUDO_CMD) buildah pull ${IMG}
	$(SUDO_CMD) buildah push ${IMG}

# Run golangci-lint against code
# TODO replace verify (except verify-generated), vet, fmt targets with lint as it covers all of it
.PHONY: lint
lint: install-tools
	golangci-lint run -c ./golangci.yml ./pkg/... ./cmd/... ./contrib/...
# Remove the golangci-lint from the verify until a fix is in place for permisions for writing to the /.cache directory.
#verify: lint

.PHONY: install-tools
install-tools:
	go install $(GO_MOD_FLAGS) github.com/golang/mock/mockgen
	go install $(GO_MOD_FLAGS) golang.org/x/lint/golint
	go install $(GO_MOD_FLAGS) github.com/golangci/golangci-lint/cmd/golangci-lint
