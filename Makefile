.PHONY: all build build-server build-ext install clean test

BINARY_NAME := phpls
BIN_DIR     := bin
SERVER_DIR  := server
GO          := go
GOFLAGS     :=

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

## build-server: compile the Go language server binary into bin/
build-server:
	@echo "==> Building Go language server..."
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
	rm -rf dist out

## dev: watch-compile TypeScript (for development)
dev:
	@echo "==> Watching TypeScript (Ctrl-C to stop)..."
	npm run watch
