SOURCES := $(shell find . -type f -name "*.go")
BRANCH  := $(shell git branch | sed -n -e 's/^\* \(.*\)/\1/p')
COMMIT  := $(shell git rev-parse HEAD)
HOST    := $(shell hostname)
GOMOD   := go.mod
GOSUM   := go.sum
VENDOR  := vendor
COVOUT  := coverage.out

BIN_DIR := target

all: deps lint test build

deps: $(GOSUM)
$(GOSUM): $(SOURCES) $(GOMOD)
	go mod tidy

LINTER_VERSION := 1.51.2
LINTER_BINARY  := $(BIN_DIR)/golangci-lint-$(LINTER_VERSION) $@

.PHONY: lint
lint: $(LINTER_BINARY)
	$(LINTER_BINARY) run $(LINT_ARGS)

$(LINTER_BINARY):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) v$(LINTER_VERSION)
	@mv $(BIN_DIR)/golangci-lint $@

.PHONY: test
test:
	go test -count=1 -cover -covermode=atomic -coverprofile=$(COVOUT) ./...

.PHONY: coverage
coverage: test
	go tool cover -html=$(COVOUT)

.PHONY: clean
clean:
	@rm -f $(COVOUT)
	@rm -rf $(VENDOR)

.PHONY: build
build:
	@go build -ldflags="-X 'main.version=dev-$(BRANCH)' -X 'main.source=$(HOST)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(shell date -u "+%FT%TZ")'" -o "$(BIN_DIR)/grafana-app-sdk" cmd/grafana-app-sdk/*.go
