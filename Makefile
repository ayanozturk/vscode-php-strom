.PHONY: all build build-server build-server-local build-ext install package publish clean test

BINARY_NAME := phpls
BIN_DIR     := bin
SERVER_DIR  := server
GO          := go
GOFLAGS     :=
TARGETS     := darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-arm64 win32-x64

# Detect OS for binary extension
ifeq ($(OS),Windows_NT)
    BINARY := $(BIN_DIR)/$(BINARY_NAME).exe
else
    BINARY := $(BIN_DIR)/$(BINARY_NAME)
endif

# VSIX package name
VSIX := $(shell node -p "require('./package.json').name + '-' + require('./package.json').version + '.vsix'" 2>/dev/null || echo "phpls-0.1.0.vsix")

all: build

## build: compile Go server + TypeScript extension
build: build-server build-ext

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
		cd $(SERVER_DIR) && GOOS="$$GOOS" GOARCH="$$GOARCH" $(GO) build $(GOFLAGS) -o "../$$OUT_DIR/$$OUT_NAME" .; \
		cd ..; \
	done

## build-server-local: compile the Go language server binary into the legacy bin/ path for local workflows
build-server-local:
	@echo "==> Building Go language server for the host platform..."
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && $(GO) build $(GOFLAGS) -o ../$(BINARY) .
	@echo "    Binary: $(BINARY)"

## build-ext: compile the TypeScript extension
build-ext:
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
package: build
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
test-server:
	@echo "==> Running Go tests..."
	cd $(SERVER_DIR) && $(GO) test ./...

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
	rm -rf dist out

## dev: watch-compile TypeScript (for development)
dev:
	@echo "==> Watching TypeScript (Ctrl-C to stop)..."
	npm run watch
