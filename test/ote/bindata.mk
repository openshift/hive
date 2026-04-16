# bindata.mk - Testdata management for OTE extension
#
# This module uses Go's embed package (via //go:embed directives in fixtures.go)
# instead of go-bindata. Testdata files are embedded at compile time from the
# hive/testdata/ directory.
#
# To add new testdata files, place them in hive/testdata/ and they will be
# automatically embedded.

TESTDATA_DIR := hive/testdata

.PHONY: list-testdata
list-testdata:
	@find $(TESTDATA_DIR) -type f | sort

.PHONY: verify-testdata
verify-testdata:
	@echo "Verifying testdata directory exists..."
	@test -d $(TESTDATA_DIR) || (echo "ERROR: $(TESTDATA_DIR) not found" && exit 1)
	@echo "Testdata directory OK: $(TESTDATA_DIR)"
	@echo "Files: $$(find $(TESTDATA_DIR) -type f | wc -l)"
