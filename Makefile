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
	$(foreach mod, $(dir $(GOMOD)), (cd $(mod) && go mod tidy -v);)

$(GOWORKSUM): $(GOWORK) $(GOMOD)
	go work sync

.PHONY: check-go-version
check-go-version:
	@if [ -z "$(GOBINARY)" ]; then \
		echo "Error: No Go binary found. It's a no-go!"; \
		exit 1; \
	fi

LINTER_VERSION := 2.5.0
LINTER_BINARY  := $(BIN_DIR)/golangci-lint-$(LINTER_VERSION)

.PHONY: lint
lint: $(LINTER_BINARY)
	$(LINTER_BINARY) run $(LINT_ARGS) $(SUBMODULES)

$(LINTER_BINARY):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) v$(LINTER_VERSION)
	@mv $(BIN_DIR)/golangci-lint $@

BENCHSTAT_VERSION := latest
BENCHSTAT_BINARY  := $(BIN_DIR)/benchstat

$(BENCHSTAT_BINARY):
	GOBIN=$(abspath $(BIN_DIR)) go install golang.org/x/perf/cmd/benchstat@$(BENCHSTAT_VERSION)

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

# Build the sdk with debug flags set (no optimizations, no inlining)
.PHONY: build/debug
build/debug: update-workspace
	@go build -gcflags="all=-N -l" -ldflags="-X 'main.version=dev-$(BRANCH)' -X 'main.source=$(HOST)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(shell date -u "+%FT%TZ")'" -o "$(BIN_DIR)/grafana-app-sdk" cmd/grafana-app-sdk/*.go

.PHONY: install
install: build
ifndef GOPATH
	@echo "GOPATH is not defined"
	exit 1
endif
	@cp "$(BIN_DIR)/grafana-app-sdk" "${GOPATH}/bin/grafana-app-sdk"

# Debug the sdk using Delve
# Requires Delve to be installed:
# 	go install github.com/go-delve/delve/cmd/dlv@latest
# Arguments:
# 	APP_DIR - Directory of the application to debug (optional, defaults to the gitignored 'target' directory where the built binary is located)
# 	PORT    - Port for Delve to listen on (optional, defaults to 12345)
# 	ARGS    - Arguments to pass to the application being debugged (optional)
# Usage:
# 	make debug APP_DIR=target/issue-tracker-project PORT=12345 ARGS='project init "github.com/grafana/issue-tracker-project"'
.PHONY: debug
debug: build/debug
	@if [ -n "$(APP_DIR)" ]; then \
		mkdir -p "$(APP_DIR)"; \
		cd "$(APP_DIR)" && dlv exec "$(abspath $(BIN_DIR))/grafana-app-sdk" --headless --listen=:$(or $(PORT),12345) --api-version=2 -- $(ARGS); \
	else \
		cd target && dlv exec "$(abspath $(BIN_DIR))/grafana-app-sdk" --headless --listen=:$(or $(PORT),12345) --api-version=2 -- $(ARGS); \
	fi

.PHONY: update-workspace
update-workspace:
	@echo "updating workspace"
	@$(foreach mod, $(dir $(GOMOD)), (cd $(mod) && go mod tidy -v);)
	go work sync
	go mod download

.PHONY: regenerate-codegen-test-files
regenerate-codegen-test-files:
	sh ./scripts/regenerate_golden_test_files.sh

.PHONY: generate
generate: build
	@$(BIN_DIR)/grafana-app-sdk generate -s=app -g=app --grouping=group --defpath=app/definitions
	rm app/manifestdata/appmanifest_manifest.go

.PHONY: bench
bench:
	@echo "Running all benchmarks..."
	go test -bench=. -benchmem -benchtime=1x -count=1 ./benchmark/

.PHONY: bench-baseline
bench-baseline: $(BENCHSTAT_BINARY)
	@echo "Establishing performance baseline..."
	@mkdir -p $(BIN_DIR)/benchmarks
	@go test -bench=. -benchmem -count=10 ./benchmark/ | tee $(BIN_DIR)/benchmarks/baseline.txt
	@echo ""
	@echo "✓ Baseline saved to $(BIN_DIR)/benchmarks/baseline.txt"
	@echo "Now make your code optimizations and run 'make bench-compare' to see the impact."

.PHONY: bench-compare
bench-compare: $(BENCHSTAT_BINARY)
	@if [ ! -f $(BIN_DIR)/benchmarks/baseline.txt ]; then \
		echo "Error: No baseline found. Please run 'make bench-baseline' first."; \
		exit 1; \
	fi
	@echo "Running benchmarks and comparing against baseline..."
	@mkdir -p $(BIN_DIR)/benchmarks
	@go test -bench=. -benchmem -count=10 ./benchmark/ | tee $(BIN_DIR)/benchmarks/current.txt
	@echo ""
	@echo "Statistical comparison (baseline vs current):"
	@echo "Note: Negative delta (%) = improvement for time/memory metrics"
	@echo ""
	@$(BENCHSTAT_BINARY) $(BIN_DIR)/benchmarks/baseline.txt $(BIN_DIR)/benchmarks/current.txt

.PHONY: bench-profile
bench-profile:
	@echo "Running benchmarks with memory profiling..."
	@mkdir -p $(BIN_DIR)/profiles
	go test -bench=. -benchmem -memprofile=$(BIN_DIR)/profiles/mem.out -cpuprofile=$(BIN_DIR)/profiles/cpu.out ./benchmark/
	@echo "Memory profile: $(BIN_DIR)/profiles/mem.out"
	@echo "CPU profile: $(BIN_DIR)/profiles/cpu.out"
	@echo "View with: go tool pprof $(BIN_DIR)/profiles/mem.out"
