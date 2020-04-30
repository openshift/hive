# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
)

GO_BUILD_BINDIR :=../bin

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor

