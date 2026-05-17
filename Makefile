.PHONY: all deps prepare-parser prepare-go-work build build-server build-server-local build-ext install package publish clean test

BINARY_NAME := phpstrom
BIN_DIR     := bin
SERVER_DIR  := server
GO          := go
NPM         := npm
GOFLAGS     :=
TARGETS     := darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-arm64 win32-x64
CACHE_DIR   := .cache
PHP_PARSER_REPO ?= https://github.com/ayanozturk/go-php-parser.git
PHP_PARSER_REF  ?= main
LOCAL_PHP_PARSER_DIR ?= ../go-php-parser
PHP_PARSER_DIR  := $(CACHE_DIR)/go-php-parser
GO_WORK_FILE    := $(abspath $(CACHE_DIR)/phpstrom.build.work)

# Detect OS for binary extension
ifeq ($(OS),Windows_NT)
    BINARY := $(BIN_DIR)/$(BINARY_NAME).exe
else
    BINARY := $(BIN_DIR)/$(BINARY_NAME)
endif

# VSIX package name
VSIX := $(shell node -p "require('./package.json').name + '-' + require('./package.json').version + '.vsix'" 2>/dev/null || echo "phpstrom-0.1.0.vsix")

all: build

## deps: install Node.js dependencies when node_modules is missing
deps:
	@if [ ! -d node_modules ]; then \
		echo "==> Installing Node.js dependencies..."; \
		$(NPM) ci; \
	fi

## build: compile Go server + TypeScript extension
build: build-server build-ext

## prepare-parser: fetch the parser dependency into a local cache when it is not already present
prepare-parser:
	@if [ -d "$(LOCAL_PHP_PARSER_DIR)/.git" ]; then \
		echo "==> Using local PHP parser sources at $(LOCAL_PHP_PARSER_DIR)"; \
		exit 0; \
	fi
	@mkdir -p $(CACHE_DIR)
	@if [ -d "$(PHP_PARSER_DIR)/.git" ]; then \
		echo "==> Reusing cached PHP parser sources at $(PHP_PARSER_DIR)"; \
	elif [ -d "$(PHP_PARSER_DIR)" ]; then \
		echo "ERROR: $(PHP_PARSER_DIR) exists but is not a git clone."; \
		exit 1; \
	else \
		echo "==> Fetching PHP parser sources from $(PHP_PARSER_REPO) ($(PHP_PARSER_REF))..."; \
		git clone --depth 1 --branch "$(PHP_PARSER_REF)" "$(PHP_PARSER_REPO)" "$(PHP_PARSER_DIR)"; \
	fi

## prepare-go-work: generate a temporary Go workspace that wires phpstrom to the cached parser module
prepare-go-work: prepare-parser
	@mkdir -p $(CACHE_DIR)
	@PARSER_PATH="$(abspath $(PHP_PARSER_DIR))"; \
	if [ -d "$(LOCAL_PHP_PARSER_DIR)/.git" ]; then PARSER_PATH="$(abspath $(LOCAL_PHP_PARSER_DIR))"; fi; \
	printf 'go 1.23\n\nuse (\n\t%s\n\t%s\n)\n' "$(abspath $(SERVER_DIR))" "$$PARSER_PATH" > "$(GO_WORK_FILE)"

## build-server: compile the Go language server binaries for all marketplace targets
build-server: prepare-go-work
	@echo "==> Building Go language server for all targets..."
	@mkdir -p $(BIN_DIR)
	@for target in $(TARGETS); do \
		PLATFORM=$${target%-*}; \
		ARCH=$${target#*-}; \
		GOOS="$$PLATFORM"; \
		GOARCH="$$ARCH"; \
		if [ "$$GOOS" = "win32" ]; then GOOS="windows"; fi; \
		if [ "$$GOARCH" = "x64" ]; then GOARCH="amd64"; fi; \
		OUT_DIR="$(BIN_DIR)/$$PLATFORM-$$ARCH"; \
		OUT_NAME="$(BINARY_NAME)"; \
		if [ "$$GOOS" = "windows" ]; then OUT_NAME="$(BINARY_NAME).exe"; fi; \
		echo "    $$PLATFORM/$$ARCH -> $$OUT_DIR/$$OUT_NAME"; \
		mkdir -p "$$OUT_DIR"; \
		cd $(SERVER_DIR) && GOWORK="$(GO_WORK_FILE)" GOOS="$$GOOS" GOARCH="$$GOARCH" $(GO) build $(GOFLAGS) -o "../$$OUT_DIR/$$OUT_NAME" .; \
		cd ..; \
	done

## build-server-local: compile the Go language server binary into the legacy bin/ path for local workflows
build-server-local: prepare-go-work
	@echo "==> Building Go language server for the host platform..."
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && GOWORK="$(GO_WORK_FILE)" $(GO) build $(GOFLAGS) -o ../$(BINARY) .
	@echo "    Binary: $(BINARY)"

## build-ext: compile the TypeScript extension
build-ext: deps
	@echo "==> Compiling TypeScript extension..."
	npm run compile

## install: build everything, package the VSIX, and install it in VS Code
install: build
	@echo "==> Packaging extension..."
	npx vsce package --no-dependencies -o $(VSIX)
	@echo "==> Installing extension in VS Code..."
	@CODE=$$(command -v code 2>/dev/null \
	  || ls "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" 2>/dev/null \
	  || ls "$$HOME/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" 2>/dev/null); \
	  if [ -z "$$CODE" ]; then echo "ERROR: 'code' CLI not found. Open VS Code → Command Palette → 'Install code command in PATH', then re-run make install."; exit 1; fi; \
	  "$$CODE" --install-extension $(VSIX)
	@echo "==> Done. Reload VS Code to activate the new version."

## package: build + produce VSIX only (no install)
package: build deps
	@echo "==> Packaging extension..."
	npx vsce package --no-dependencies -o $(VSIX)
	@echo "    Package: $(VSIX)"

## publish: build, package, and publish the VSIX to the VS Code Marketplace
publish: package
	@if [ -z "$$VSCE_PAT" ]; then \
		echo "ERROR: VSCE_PAT is not set. Export your Marketplace token and re-run make publish."; \
		exit 1; \
	fi
	@echo "==> Publishing extension to Marketplace..."
	npx vsce publish --packagePath $(VSIX)
	@echo "==> Published $(VSIX)"

## test-server: run Go unit tests
test-server: prepare-go-work
	@echo "==> Running Go tests..."
	cd $(SERVER_DIR) && GOWORK="$(GO_WORK_FILE)" $(GO) test ./...

## test-ext: run TypeScript/VS Code extension tests
test-ext:
	@echo "==> Running extension tests..."
	npm test

## test: run all tests
test: test-server test-ext

## clean: remove build artefacts
clean:
	@echo "==> Cleaning..."
	rm -f $(BINARY) $(BIN_DIR)/$(BINARY_NAME).exe *.vsix
	rm -rf $(BIN_DIR)/darwin-arm64 $(BIN_DIR)/darwin-x64 $(BIN_DIR)/linux-arm64 $(BIN_DIR)/linux-x64 $(BIN_DIR)/win32-arm64 $(BIN_DIR)/win32-x64
	rm -rf $(CACHE_DIR) dist out

## dev: watch-compile TypeScript (for development)
dev:
	@echo "==> Watching TypeScript (Ctrl-C to stop)..."
	npm run watch
