SOURCES 	:= $(shell find . -type f -name "*.go")
BRANCH  	:= $(shell git branch | sed -n -e 's/^\* \(.*\)/\1/p')
COMMIT  	:= $(shell git rev-parse HEAD)
HOST    	:= $(shell hostname)
GOMOD   	:= $(shell find . -type f -name "go.mod")
GOSUM   	:= $(shell find . -type f -name "go.sum")
SUBMODULES 	:= $(foreach d,$(dir $(GOMOD)),$(d)...)
GOWORK		:= go.work
GOWORKSUM 	:= go.work.sum
VENDOR  	:= vendor
COVOUT  	:= coverage.out
GOVERSION   := $(shell awk '/^go / {print $$2}' go.mod)
GOBINARY    := $(shell which go)

BIN_DIR := target

all: check-go-version deps lint test build

deps: $(GOSUM) $(GOWORKSUM)
$(GOSUM): $(SOURCES) $(GOMOD)
	$(foreach mod, $(dir $(GOMOD)), pushd . && cd $(mod) && go mod tidy -v; popd;)

$(GOWORKSUM): $(GOWORK) $(GOMOD)
	go work sync

.PHONY: check-go-version
check-go-version:
	@if [ -z "$(GOBINARY)" ]; then \
		echo "Error: No Go binary found. It's a no-go!"; \
		exit 1; \
	fi

LINTER_VERSION := 1.62.2
LINTER_BINARY  := $(BIN_DIR)/golangci-lint-$(LINTER_VERSION)

.PHONY: lint
lint: $(LINTER_BINARY)
	$(LINTER_BINARY) run $(LINT_ARGS) $(SUBMODULES)

$(LINTER_BINARY):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) v$(LINTER_VERSION)
	@mv $(BIN_DIR)/golangci-lint $@

.PHONY: test
test:
	go test -count=1 -cover -covermode=atomic -coverprofile=$(COVOUT) $(SUBMODULES)

.PHONY: coverage
coverage: test
	go tool cover -html=$(COVOUT)

.PHONY: clean
clean:
	@rm -f $(COVOUT)
	@rm -rf $(VENDOR)

.PHONY: build
build: update-workspace
	@go build -ldflags="-X 'main.version=dev-$(BRANCH)' -X 'main.source=$(HOST)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(shell date -u "+%FT%TZ")'" -o "$(BIN_DIR)/grafana-app-sdk" cmd/grafana-app-sdk/*.go

.PHONY: install
install: build
ifndef GOPATH
	@echo "GOPATH is not defined"
	exit 1
endif
	@cp "$(BIN_DIR)/grafana-app-sdk" "${GOPATH}/bin/grafana-app-sdk"

.PHONY: update-workspace
update-workspace:
	@echo "updating workspace"
	@$(foreach mod, $(dir $(GOMOD)), pushd . && cd $(mod) && go mod tidy -v; popd;)
	go work sync
	go mod download

.PHONY: regenerate-codegen-test-files
regenerate-codegen-test-files:
	sh ./scripts/regenerate_golden_test_files.sh
