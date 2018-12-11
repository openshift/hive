BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go')
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml
DOCKER_CMD ?= docker



# Image URL to use all building/pushing image targets
IMG ?= hive-controller:latest

all: fmt vet test build

# Run tests
.PHONY: test
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... ./contrib/... -coverprofile cover.out

test-integration: generate
	go test ./test/integration/... -coverprofile cover.out

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
deploy: manifests docker-build deploy-hiveadmission
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy-sd-dev
deploy-sd-dev: manifests deploy-hiveadmission
	kubectl apply -f config/crds
	kustomize build config/overlays/sd-dev | kubectl apply -f -

.PHONY: deploy-hiveadmission
deploy-hiveadmission:
	$(eval kube_service_account := $(shell oc get secret -n kube-system | awk "/kubernetes.io\/service-account-token/ { line=\$$1 } END{print line}"))
	$(eval service_ca := $(shell oc get secret -n openshift-service-cert-signer "service-serving-cert-signer-signing-key" -o json | jq -r '.data."tls.crt"'))
	$(eval kube_ca := $(shell oc get secret -n kube-system "$(kube_service_account)" -o json | jq -r '.data."ca.crt"'))
	@oc process -f config/templates/hiveadmission.yaml SERVICE_CA="$(service_ca)" KUBE_CA="$(kube_ca)" | oc apply -n openshift-hive -f -
	@oc apply -f config/rbac/hiveadmission_rbac_role.yaml -f config/rbac/hiveadmission_rbac_role_binding.yaml

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
docker-build: manager hiveutil hiveadmission
	$(eval build_path := ./build/hive)
	$(eval tmp_build_path := "$(build_path)/tmp")
	mkdir -p $(tmp_build_path)
	cp $(build_path)/Dockerfile $(tmp_build_path)
	cp ./bin/* $(tmp_build_path)
	$(DOCKER_CMD) build -t ${IMG} $(tmp_build_path)
	rm -rf $(tmp_build_path)

# Push the docker image
.PHONY: docker-push
docker-push:
	$(DOCKER_CMD) push ${IMG}

# Build the image with buildah
.PHONY: buildah-build
buildah-build: manager hiveutil hiveadmission
	$(eval build_path := ./build/hive)
	$(eval tmp_build_path := "$(build_path)/tmp")
	mkdir -p $(tmp_build_path)
	cp $(build_path)/Dockerfile $(tmp_build_path)
	cp ./bin/* $(tmp_build_path)
	BUILDAH_ISOLATION=chroot sudo buildah bud --tag ${IMG} $(tmp_build_path)
	rm -rf $(tmp_build_path)
