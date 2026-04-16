EXTENSION_NAME := hive
EXTENSION_BIN := bin/$(EXTENSION_NAME)
CMD_DIR := cmd/extension

.PHONY: all
all: build

.PHONY: build
build:
	go build -o $(EXTENSION_BIN) ./$(CMD_DIR)/...

.PHONY: test
test:
	go test ./...

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor

.PHONY: verify
verify:
	go vet ./...

.PHONY: clean
clean:
	rm -rf bin/

.PHONY: list
list: build
	./$(EXTENSION_BIN) list

.PHONY: run
run: build
	./$(EXTENSION_BIN) run
