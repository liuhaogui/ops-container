APP_NAME := ops-container
MAIN := ./cmd/server
BUILD_DIR := ./bin
BUILD_BIN := $(BUILD_DIR)/$(APP_NAME)
SWAG_MAIN := ./cmd/server/main.go
SWAG_PKG := github.com/swaggo/swag/cmd/swag@v1.16.4
AIR_PKG := github.com/air-verse/air@latest

.PHONY: help check-go check-air check-swag tidy run build clean swagger dev air-install swag-install init-tools

help:
	@echo "make tidy        - download and tidy go modules"
	@echo "make run         - run server locally"
	@echo "make build       - build binary to ./bin/ops-container"
	@echo "make clean       - remove build artifacts"
	@echo "make swagger     - generate swagger docs"
	@echo "make air-install - install air hot reload tool"
	@echo "make swag-install - install swag tool"
	@echo "make init-tools   - install both air and swag"
	@echo "make dev         - run hot reload development mode"

check-go:
	@command -v go >/dev/null 2>&1 || (echo "go not found in PATH. Please install Go and ensure 'go' is available."; exit 1)

check-air:
	@command -v air >/dev/null 2>&1 || (echo "air not found in PATH. Run 'make air-install' first."; exit 1)

check-swag:
	@command -v swag >/dev/null 2>&1 || echo "swag not found in PATH, will use 'go run $(SWAG_PKG)'"

tidy: check-go
	go mod tidy

run: check-go
	go run $(MAIN)

build: check-go
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_BIN) $(MAIN)

clean:
	rm -rf $(BUILD_DIR) ./tmp

swagger: check-swag
	go run $(SWAG_PKG) init -g $(SWAG_MAIN) -o ./docs

air-install: check-go
	go install $(AIR_PKG)

swag-install: check-go
	go install $(SWAG_PKG)

init-tools: air-install swag-install

dev: check-air
	air -c .air.toml
