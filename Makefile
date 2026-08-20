.PHONY: all deps prepare-go-work build build-server build-server-local build-server-dev build-ext install package publish release clean test test-server test-server-dev

BINARY_NAME := phpstrom
BIN_DIR     := bin
SERVER_DIR  := server
GO          := go
NPM         := npm
MAKE_CMD    := $(MAKE)
VERSION_BUMP ?= patch
GOFLAGS     :=
TARGETS     := darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-arm64 win32-x64
CACHE_DIR   := .cache
LOCAL_PHP_PARSER_DIR ?= ../go-php-parser
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

## prepare-go-work: generate a temporary Go workspace for explicit sibling-parser development
prepare-go-work:
	@if [ ! -f "$(LOCAL_PHP_PARSER_DIR)/go.mod" ]; then \
		echo "ERROR: local parser module not found at $(LOCAL_PHP_PARSER_DIR)."; \
		exit 1; \
	fi
	@mkdir -p $(CACHE_DIR)
	@printf 'go 1.23\n\nuse (\n\t%s\n\t%s\n)\n' "$(abspath $(SERVER_DIR))" "$(abspath $(LOCAL_PHP_PARSER_DIR))" > "$(GO_WORK_FILE)"

## build-server: compile the Go language server binaries for all marketplace targets
build-server:
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
		cd $(SERVER_DIR) && GOWORK=off GOOS="$$GOOS" GOARCH="$$GOARCH" $(GO) build $(GOFLAGS) -o "../$$OUT_DIR/$$OUT_NAME" .; \
		cd ..; \
	done

## build-server-local: compile the host binary using the parser version pinned in server/go.mod
build-server-local:
	@echo "==> Building Go language server for the host platform..."
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && GOWORK=off $(GO) build $(GOFLAGS) -o ../$(BINARY) .
	@echo "    Binary: $(BINARY)"

## build-server-dev: compile the host binary against an explicit sibling parser checkout
build-server-dev: prepare-go-work
	@echo "==> Building Go language server against $(LOCAL_PHP_PARSER_DIR)..."
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && GOWORK="$(GO_WORK_FILE)" $(GO) build $(GOFLAGS) -o ../$(BINARY) .

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

## release: bump the extension version, build and install it, then commit the version metadata
release:
	@set -eu; \
	case "$(VERSION_BUMP)" in \
		major|minor|patch|premajor|preminor|prepatch|prerelease) ;; \
		*) echo "ERROR: VERSION_BUMP must be one of major, minor, patch, premajor, preminor, prepatch, or prerelease."; exit 1 ;; \
	esac; \
	if ! git diff --quiet -- package.json package-lock.json || ! git diff --cached --quiet -- package.json package-lock.json; then \
		echo "ERROR: package.json or package-lock.json has uncommitted changes."; \
		exit 1; \
	fi; \
	OLD_VERSION=$$(node -p "require('./package.json').version"); \
	$(NPM) version "$(VERSION_BUMP)" --no-git-tag-version; \
	NEW_VERSION=$$(node -p "require('./package.json').version"); \
	$(MAKE_CMD) install; \
	git add package.json package-lock.json; \
	git diff --cached --check; \
	git commit -m "Bump extension version to $$NEW_VERSION"; \
	echo "==> Released $$OLD_VERSION -> $$NEW_VERSION"

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

## test-server: run Go unit tests against the parser version pinned in server/go.mod
test-server:
	@echo "==> Running Go tests..."
	cd $(SERVER_DIR) && GOWORK=off $(GO) test ./...

## test-server-dev: run Go unit tests against an explicit sibling parser checkout
test-server-dev: prepare-go-work
	@echo "==> Running Go tests against $(LOCAL_PHP_PARSER_DIR)..."
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
