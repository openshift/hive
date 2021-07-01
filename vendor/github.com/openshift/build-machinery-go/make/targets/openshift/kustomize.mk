# We need to include this before we use PERMANENT_TMP_GOPATH
# (indirectly) from ifeq.
include $(addprefix $(dir $(lastword $(MAKEFILE_LIST))), \
	../../lib/tmp.mk \
)

KUSTOMIZE_VERSION ?= 4.1.3
KUSTOMIZE ?= $(PERMANENT_TMP_GOPATH)/bin/kustomize
kustomize_dir := $(dir $(KUSTOMIZE))

ensure-kustomize:
ifeq "" "$(wildcard $(KUSTOMIZE))"
	$(info Installing kustomize into '$(KUSTOMIZE)')
	mkdir -p '$(kustomize_dir)'
	@# NOTE: Pinning script to a tag rather than `master` for security reasons
	curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/kustomize/v$(KUSTOMIZE_VERSION)/hack/install_kustomize.sh"  | bash -s $(KUSTOMIZE_VERSION) $(kustomize_dir)
else
	$(info Using existing kustomize from "$(KUSTOMIZE)")
endif
.PHONY: ensure-kustomize

clean-kustomize:
	$(RM) '$(KUSTOMIZE)'
	if [ -d '$(kustomize_dir)' ]; then rmdir --ignore-fail-on-non-empty -p '$(kustomize_dir)'; fi
.PHONY: clean-kustomize

clean: clean-kustomize
