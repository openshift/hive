BINDIR = bin
SRC_DIRS = pkg contrib
GOFILES = $(shell find $(SRC_DIRS) -name '*.go')
VERIFY_IMPORTS_CONFIG = build/verify-imports/import-rules.yaml



# Image URL to use all building/pushing image targets
IMG ?= hive-controller:latest

all: fmt vet test build

# Run tests
.PHONY: test
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Builds all of hive's binaries (including utils).
.PHONY: build
build: manager hiveutil


# Build manager binary
manager: generate
	go build -o bin/manager github.com/openshift/hive/cmd/manager

# Build hiveutil binary
hiveutil:
	go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil

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
deploy: manifests docker-build
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

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
docker-build: manager hiveutil
	$(eval build_path := ./build/hive)
	$(eval tmp_build_path := "$(build_path)/tmp")
	mkdir -p $(tmp_build_path)
	cp $(build_path)/Dockerfile $(tmp_build_path)
	cp ./bin/* $(tmp_build_path)
	docker build -t ${IMG} $(tmp_build_path)
	rm -rf $(tmp_build_path)

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}
