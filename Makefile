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

BIN_DIR := target

all: deps lint test build

deps: $(GOSUM) $(GOWORKSUM)
$(GOSUM): $(SOURCES) $(GOMOD)
	$(foreach mod, $(dir GOMOD), cd $(mod) && go mod tidy -v;)

$(GOWORKSUM): $(GOWORK) $(GOMOD)
	go work sync

LINTER_VERSION := 1.60.3
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
	go mod download
