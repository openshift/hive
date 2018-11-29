all: build
.PHONY: all

build:
	go build -o _output/bin/generic-admission-server github.com/openshift/generic-admission-server/pkg/cmd
.PHONY: build

clean:
	rm -rf _output
.PHONY: clean

update-deps:
	hack/update-deps.sh
.PHONY: generate
